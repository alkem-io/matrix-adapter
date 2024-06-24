import { Controller, Get } from '@nestjs/common';

@Controller('/health')
export class HealthController {
  @Get('/')
  public async getHello(): Promise<string> {
    return 'healthy!';
  }
}
