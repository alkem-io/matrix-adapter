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

    // Only check circuit state if we're not on the first attempt
    if (attempt > 1) {
      if (this.state === "OPEN") {
        const timeSinceFailure = Date.now() - this.lastFailureTime;
        if (timeSinceFailure < this.resetTimeout) {
          const remainingTime = this.resetTimeout - timeSinceFailure;
          this.logger.warn?.(
            `Circuit breaker is OPEN for room ${roomId}. Blocking request. Retry after ${remainingTime}ms (${Math.round(remainingTime / 1000)}s)`,
            LogContext.COMMUNICATION
          );
          throw new Error(
            `Circuit breaker is open for room ${roomId}. ` +
            `Retry after ${remainingTime}ms`
          );
        } else {
          this.logger.verbose?.(
            `Circuit breaker transitioning from OPEN to HALF_OPEN for room ${roomId} - Allowing test request`,
            LogContext.COMMUNICATION
          );
          this.state = "HALF_OPEN";
        }
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

      // Only open circuit if we've exhausted all attempts
      if (attempt >= maxAttempts && this.failureCount >= this.failureThreshold) {
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