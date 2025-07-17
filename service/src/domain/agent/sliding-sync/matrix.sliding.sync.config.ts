export interface SlidingSyncConfig {
  windowSize: number;
  sortOrder: 'activity' | 'alphabetical' | 'unread';
  includeEmptyRooms: boolean;
  ranges: [number, number][];
}
