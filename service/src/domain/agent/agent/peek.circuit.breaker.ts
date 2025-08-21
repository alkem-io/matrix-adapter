import { LoggerService } from '@nestjs/common';
import { LogContext } from '@src/common/enums/logging.context';

type CircuitBreakerState = "CLOSED" | "OPEN" | "HALF_OPEN";

export interface CircuitBreakerConfig {
  failureThreshold: number;
  initialTimeout: number;
  maxTimeout: number;
  maxRetries: number;
}

export class PeekCircuitBreaker {
  private state: CircuitBreakerState = "CLOSED";
  private failureCount = 0;
  private lastFailureTime = 0;
  private readonly failureThreshold: number;
  private resetTimeout: number;
  private readonly maxResetTimeout: number;
  private readonly maxRetries: number;

  constructor(
    private readonly logger: LoggerService,
    private readonly config: CircuitBreakerConfig
  ) {
    this.failureThreshold = config.failureThreshold;
    this.resetTimeout = config.initialTimeout;
    this.maxResetTimeout = config.maxTimeout;
    this.maxRetries = config.maxRetries;

    this.logger.verbose?.(
      `Circuit breaker initialized with failure threshold: ${this.failureThreshold}, initial timeout: ${this.resetTimeout}ms, max timeout: ${this.maxResetTimeout}ms, max retries: ${this.maxRetries}`,
      LogContext.COMMUNICATION
    );
  }

  // Helper to detect errors that should not be retried (Matrix-403 forbidden peeks)
  private isNonRetryable(error: unknown): boolean {
    const err: any = error as any;
    // Normalize status (number or numeric string)
    const raw = err?.httpStatus ?? err?.statusCode ?? err?.status;
    const status = typeof raw === 'string' ? Number(raw) : raw;
    // Matrix errcode from body or top-level error
    const body = err?.body ?? err?.data;
    const errcode: string | undefined =
      (typeof body?.errcode === 'string' && body.errcode) ||
      (typeof err?.errcode === 'string' && err.errcode);
    // Primary human message fallback
    const message: string =
      (typeof err?.message === 'string' && err.message) ||
      (typeof body?.error === 'string' && body.error) ||
      '';
    const is403 = status === 403;
    const forbiddenErrcode =
      errcode === 'M_FORBIDDEN' || errcode === 'M_GUEST_ACCESS_FORBIDDEN';
    const notJoined = /not in room|not joined/i.test(message);
    const accessDisabled =
      /previews?\s+are\s+disabled|guest access is disabled|world readable is disabled/i.test(message);
    // Conservative: require HTTP 403 and either an explicit errcode or both textual hints
    return Boolean(is403 && (forbiddenErrcode || (notJoined && accessDisabled)));
  }

  public async attemptPeek<T>(
    fn: () => Promise<T>,
    roomId: string,
    attempt: number,
    maxAttempts: number
  ): Promise<T> {
    const startTime = Date.now();

    this.logger.verbose?.(
      `Circuit breaker attempt ${attempt}/${maxAttempts} for room ${roomId} - State: ${this.state}, Failure count: ${this.failureCount}`,
      LogContext.COMMUNICATION
    );

    // Always enforce circuit state before executing the function
    if (this.state === "OPEN") {
      const timeSinceFailure = Date.now() - this.lastFailureTime;
      if (timeSinceFailure < this.resetTimeout) {
        const remainingTime = this.resetTimeout - timeSinceFailure;
        this.logger.warn?.(
          `Circuit breaker is OPEN for room ${roomId}. Blocking request. Retry after ${remainingTime}ms (${Math.round(remainingTime / 1000)}s)`,
          LogContext.COMMUNICATION
        );
        throw new Error(
          `Circuit breaker is open for room ${roomId}. Retry after ${remainingTime}ms`
        );
      } else {
        this.logger.verbose?.(
          `Circuit breaker transitioning from OPEN to HALF_OPEN for room ${roomId} - Allowing test request`,
          LogContext.COMMUNICATION
        );
        this.state = "HALF_OPEN";
      }
    }

    try {
      const result = await fn();
      const duration = Date.now() - startTime;

      // Reset on success
      if (this.state !== "CLOSED") {
        this.logger.verbose?.(
          `Circuit breaker SUCCESS - Transitioning to CLOSED for room ${roomId} (took ${duration}ms)`,
          LogContext.COMMUNICATION
        );
        this.state = "CLOSED";
        this.failureCount = 0;
        this.resetTimeout = this.config.initialTimeout; // Reset timeout on success
      } else {
        this.logger.verbose?.(
          `Circuit breaker request successful for room ${roomId} (took ${duration}ms)`,
          LogContext.COMMUNICATION
        );
      }

      return result;
    } catch (error) {
      const duration = Date.now() - startTime;
      this.failureCount++;
      this.lastFailureTime = Date.now();

      this.logger.warn?.(
        `Circuit breaker FAILURE ${this.failureCount}/${this.failureThreshold} for room ${roomId} (took ${duration}ms) - Error: ${error instanceof Error ? error.message : 'Unknown error'}`,
        LogContext.COMMUNICATION
      );

      // Non-retryable error handling: immediately open the circuit and use max reset timeout
      if (this.isNonRetryable(error)) {
        const previousState = this.state;
        this.state = "OPEN";
        // Cap the counter to the threshold to avoid unbounded logs like 571/5
        this.failureCount = Math.max(this.failureCount, this.failureThreshold);
        const oldTimeout = this.resetTimeout;
        this.resetTimeout = this.maxResetTimeout;
        this.logger.error?.(
          `Circuit breaker OPENING for room ${roomId} due to non-retryable error (403 not in room / previews disabled). ` +
          `Previous state: ${previousState}. Timeout ${oldTimeout}ms → ${this.resetTimeout}ms. Next retry allowed in ${this.resetTimeout}ms (${Math.round(this.resetTimeout / 1000)}s)`,
          LogContext.COMMUNICATION
        );
        throw error;
      }

      // If the test request in HALF_OPEN fails, immediately re-open
      if (this.state === "HALF_OPEN") {
        const previousState = this.state;
        this.state = "OPEN";
        this.logger.error?.(
          `Circuit breaker RE-OPENING from HALF_OPEN for room ${roomId} after failed test request. ` +
          `Previous state: ${previousState}. Next retry allowed in ${this.resetTimeout}ms (${Math.round(this.resetTimeout / 1000)}s)`,
          LogContext.COMMUNICATION
        );
        throw error;
      }

      // Only apply backoff if we're not on the last attempt
      if (attempt < maxAttempts) {
        const oldTimeout = this.resetTimeout;
        this.resetTimeout = Math.min(
          this.maxResetTimeout,
          this.resetTimeout * 2
        );

        if (oldTimeout !== this.resetTimeout) {
          this.logger.verbose?.(
            `Circuit breaker applying exponential backoff for room ${roomId}: ${oldTimeout}ms → ${this.resetTimeout}ms`,
            LogContext.COMMUNICATION
          );
        }
      }

      // Open the circuit as soon as the failure threshold is reached, independent of attempts
      if (this.failureCount >= this.failureThreshold) {
        const previousState = this.state;
        this.state = "OPEN";
        this.logger.error?.(
          `Circuit breaker OPENING for room ${roomId} - Failure threshold reached (${this.failureCount}/${this.failureThreshold}). ` +
          `Previous state: ${previousState}. Next retry allowed in ${this.resetTimeout}ms (${Math.round(this.resetTimeout / 1000)}s)`,
          LogContext.COMMUNICATION
        );
      }

      throw error;
    }
  }

  /**
   * Get the maximum number of retries configured
   */
  public getMaxRetries(): number {
    return this.maxRetries;
  }

  /**
   * Get current circuit breaker status for monitoring/debugging
   */
  public getStatus() {
    const timeSinceLastFailure = this.lastFailureTime ? Date.now() - this.lastFailureTime : null;
    const nextRetryAllowedIn = this.state === "OPEN" && timeSinceLastFailure !== null
      ? Math.max(0, this.resetTimeout - timeSinceLastFailure)
      : 0;

    return {
      state: this.state,
      failureCount: this.failureCount,
      failureThreshold: this.failureThreshold,
      resetTimeout: this.resetTimeout,
      maxResetTimeout: this.maxResetTimeout,
      lastFailureTime: this.lastFailureTime,
      timeSinceLastFailure,
      nextRetryAllowedIn,
      isHealthy: this.state === "CLOSED" && this.failureCount === 0
    };
  }

  /**
   * Log current circuit breaker status
   */
  public logStatus(roomId?: string) {
    const status = this.getStatus();
    const roomInfo = roomId ? ` for room ${roomId}` : '';

    this.logger.verbose?.(
      `Circuit breaker status${roomInfo}: ${JSON.stringify(status, null, 2)}`,
      LogContext.COMMUNICATION
    );
  }
}