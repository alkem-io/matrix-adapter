import { URL } from 'url';

import { ConfigurationTypes } from '@common/enums/configuration.type';
import { MatrixUserLoginException } from '@common/exceptions/matrix.login.exception';
import { MatrixUserRegistrationException } from '@common/exceptions/matrix.registration.exception';
import { HttpService } from '@nestjs/axios';
import pkg from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import { LogContext } from '@src/common/enums/logging.context';
import { SynapseEndpoint } from '@src/common/enums/synapse.endpoint';
import { AlkemioMatrixLogger } from '@src/core/logger/matrix.logger';
import { MatrixUserAdapter } from '@src/domain/adapter-user/matrix.user.adapter';
import { IMatrixUser } from '@src/domain/user/matrix.user.interface';
import { MatrixCryptographyService } from '@src/services/cryptography/matrix.cryptography.service';
import { AxiosError } from 'axios';
import { createClient,ICreateClientOpts, MatrixClient } from 'matrix-js-sdk';
import { WINSTON_MODULE_NEST_PROVIDER } from 'nest-winston';
const { Inject, Injectable } = pkg;

@Injectable()
export class MatrixAdminUserService {
  _matrixClient: MatrixClient;
  idBaseUrl: string;
  baseUrl: string;

  constructor(
    @Inject(WINSTON_MODULE_NEST_PROVIDER)
    private readonly logger: pkg.LoggerService,
    private configService: ConfigService,
    private cryptographyService: MatrixCryptographyService,
    private matrixUserAdapter: MatrixUserAdapter,
    private httpService: HttpService
  ) {
    this.idBaseUrl = this.configService.get(
      ConfigurationTypes.MATRIX
    )?.server?.url;
    this.baseUrl = this.configService.get(
      ConfigurationTypes.MATRIX
    )?.server?.url;

    if (!this.idBaseUrl || !this.baseUrl) {
      throw new Error('Matrix configuration is not provided');
    }

    // Create a single instance of the matrix client - non authenticated
    // Todo: should be handed (injected) to this client instance externally!
    const timelineSupport: boolean = this.configService.get(
      ConfigurationTypes.MATRIX
    )?.client.timelineSupport;
    this.logger.verbose?.(
      `Creating Matrix Client for management using timeline flag: ${timelineSupport}`,
      LogContext.MATRIX
    );
    const alkemioMatrixLogger = new AlkemioMatrixLogger(this.logger);

    const createClientInput: ICreateClientOpts = {
      baseUrl: this.baseUrl,
      idBaseUrl: this.idBaseUrl,
      timelineSupport: timelineSupport,
      logger: alkemioMatrixLogger,
    };
    this._matrixClient = createClient(createClientInput);
  }

  public async acquireMatrixUser(matrixUserID: string) {
    const isRegistered = await this.isRegistered(matrixUserID);

    if (isRegistered) {
      return await this.login(matrixUserID);
    }

    return await this.register(matrixUserID);
  }

  async register(
    matrixUserID: string,
    password?: string,
    isAdmin = false
  ): Promise<IMatrixUser> {
    const url = new URL(SynapseEndpoint.REGISTRATION, this.baseUrl);
    const user = this.matrixUserAdapter.convertMatrixIDToMatrixUser(
      matrixUserID,
      password
    );

    const nonceResponse = await this.httpService
      .get<{ nonce: string }>(url.href)
      .toPromise();

    if (!nonceResponse)
      throw new MatrixUserRegistrationException(
        'Invalid nonce response!',
        LogContext.COMMUNICATION
      );

    const nonce = nonceResponse.data['nonce'];
    const hmac = this.cryptographyService.generateHmac(user, nonce, isAdmin);

    const registerBody = {
      nonce,
      username: user.name,
      password: user.password,
      bind_emails: [],
      admin: isAdmin,
      mac: hmac,
    };

    const registrationResponse = await this.httpService
      .post<{ user_id: string; access_token: string }>(url.href, registerBody)
      .toPromise()
      .catch((err: unknown) => {
        const error = err as AxiosError;
        this.logger.error(
          `Matrix user registration failed: ${error.message}${
            error.response && JSON.stringify(error.response?.data)
          }`
        );
      });

    if (!registrationResponse)
      throw new MatrixUserRegistrationException(
        'Invalid registration response!',
        LogContext.COMMUNICATION
      );

    if (
      registrationResponse?.status > 400 &&
      registrationResponse.status < 600
    ) {
      throw new MatrixUserRegistrationException(
        `Unsuccessful registration for ${JSON.stringify(user)}`,
        LogContext.COMMUNICATION
      );
    }

    // Check to ensure that the response home server matches the request
    if (registrationResponse.data.user_id !== user.username) {
      throw new MatrixUserRegistrationException(
        `Registration response user_id '${registrationResponse.data.user_id}' is not equal to the supplied username: ${user.username} - please check configuration`,
        LogContext.COMMUNICATION
      );
    }

    const operationalUser = {
      name: user.name,
      password: user.password,
      username: registrationResponse.data.user_id,
      accessToken: registrationResponse.data.access_token,
    };
    let msg = 'User registration';
    if (isAdmin) msg = 'Admin registration';
    this.matrixUserAdapter.logMatrixUser(operationalUser, msg);
    return operationalUser;
  }

  async login(
    matrixUserID: string,
    password?: string
  ): Promise<IMatrixUser> {
    const matrixUser = this.matrixUserAdapter.convertMatrixIDToMatrixUser(
      matrixUserID,
      password
    );

    try {
      const operationalUser = await this._matrixClient.loginWithPassword(
        matrixUser.username,
        matrixUser.password
      );
      return {
        name: matrixUser.name,
        password: matrixUser.password,
        username: operationalUser.user_id,
        accessToken: operationalUser.access_token,
      };
    } catch (error) {
      this.matrixUserAdapter.logMatrixUser(matrixUser, 'login failed');
      throw new MatrixUserLoginException(
        `Error: ${error}`,
        LogContext.COMMUNICATION
      );
    }
  }

  //@Profiling.asyncApi
  async isRegistered(matrixUserID: string): Promise<boolean> {
    const username =
      this.matrixUserAdapter.convertMatrixIDToUsername(matrixUserID);
    /*try {
      const username =
        this.matrixUserAdapter.convertMatrixIDToUsername(matrixUserID);
      const isUsernameAvailable = await this._matrixClient.isUsernameAvailable(username);
      return false;
    } catch (error: any) {
      const errcode = error.errcode;
      if (errcode === 'M_USER_IN_USE') {
        return true;
      }
      this.logger.error(
        `Unable to check if username is available: ${error}`,
        LogContext.COMMUNICATION
      );
      throw error;
    }*/
    try {
      const isUsernameAvailable =
        await this._matrixClient.isUsernameAvailable(username);
      return !isUsernameAvailable;
    } catch (error: unknown) {
      this.logger.error(
        `Unable to check if username is available: ${error}`,
        LogContext.COMMUNICATION
      );
      return false;
    }
  }
}
