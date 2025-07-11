export interface MatrixSlidingSyncConfig {
  enabled: boolean;
  windowSize: number;
  sortOrder: 'activity' | 'alphabetical' | 'unread';
  includeEmptyRooms: boolean;
  ranges: [number, number][];
  fallbackToFullSync: boolean;
}

export interface MatrixClientConfig {
  timelineSupport: boolean;
  startupPollTimeout: number;
  startupInitialSyncLimit: number;
  slidingSync: MatrixSlidingSyncConfig;
}

export interface MatrixServerConfig {
  url: string;
}

export interface MatrixConfig {
  server: MatrixServerConfig;
  client: MatrixClientConfig;
}
