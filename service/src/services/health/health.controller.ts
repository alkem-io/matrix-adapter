import { Controller, Get } from '@nestjs/common';

@Controller('/health')
export class HealthController {
  @Get('/')
  public getHello(): string {
    return 'healthy!';
  }
}
