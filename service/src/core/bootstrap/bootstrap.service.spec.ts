import { Test, TestingModule } from '@nestjs/testing';
import { MockWinstonProvider } from '@test/mocks';
import { defaultMockerFactory } from '@test/utils';
import { MockConfigServiceProvider } from '@test/mocks';
import { BootstrapService } from './bootstrap.service';

describe('BootstrapService', () => {
  let service: BootstrapService;

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      providers: [
        BootstrapService,
        MockConfigServiceProvider,
        MockWinstonProvider,
      ],
    })
      .useMocker(defaultMockerFactory)
      .compile();

    service = module.get(BootstrapService);
  });

  it('should be defined', () => {
    expect(service).toBeDefined();
  });
});
