import { ConfigurationTypes } from '@common/enums/configuration.type';
import { Injectable } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import * as crypto from 'crypto';
import { IMatrixUser } from '../adapter-user/matrix.user.interface';
@Injectable()
export class MatrixCryptographyService {
  constructor(private configService: ConfigService) {}

  generateHmac(user: IMatrixUser, nonce: string, isAdmin?: boolean): string {
    const sharedSecret = this.configService.get(ConfigurationTypes.MATRIX)
      ?.server?.shared_secret;

    if (!sharedSecret) {
      throw new Error('Matrix configuration is not provided');
    }

    // Prepare the message as a Buffer
    const parts = [
      nonce,
      '\x00',
      user.name,
      '\x00',
      user.password,
      '\x00',
      isAdmin ? 'admin' : 'notadmin',
    ];
    // Join and encode as utf8
    const message = Buffer.from(parts.join(''), 'utf8');
    const hmac = crypto.createHmac('sha1', Buffer.from(sharedSecret, 'utf8'));
    hmac.update(message);
    return hmac.digest('hex');
  }
}
