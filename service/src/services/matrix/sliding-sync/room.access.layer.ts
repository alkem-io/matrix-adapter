import { Injectable } from '@nestjs/common';
import { Room, MatrixClient } from 'matrix-js-sdk';
import { SlidingWindowManager } from './sliding.window.manager';

export interface IRoomAccessLayer {
  getRoomAsync(roomId: string): Promise<Room | null>;
  ensureRoomLoaded(roomId: string): Promise<void>;
  isRoomInWindow(roomId: string): boolean;
  getRoomList(offset: number, limit: number): Promise<Room[]>;
  getRoomSummary(roomId: string): Promise<RoomSummary>;
  getTotalRoomCount(): Promise<number>;
}

export interface RoomSummary {
  roomId: string;
  name?: string;
  unreadCount: number;
  notificationCount: number;
  lastActivity: Date;
}

@Injectable()
export class RoomAccessLayer implements IRoomAccessLayer {
  constructor(
    private slidingWindowManager: SlidingWindowManager,
    private matrixClient: MatrixClient
  ) {}

  async getRoomAsync(roomId: string): Promise<Room | null> {
    return await this.slidingWindowManager.ensureRoomLoaded(roomId);
  }

  async ensureRoomLoaded(roomId: string): Promise<void> {
    await this.slidingWindowManager.ensureRoomLoaded(roomId);
  }

  isRoomInWindow(roomId: string): boolean {
    return this.slidingWindowManager.isRoomInWindow(roomId);
  }

  async getRoomList(offset: number, limit: number): Promise<Room[]> {
    return await this.slidingWindowManager.getRoomList(offset, limit);
  }

  async getRoomSummary(roomId: string): Promise<RoomSummary> {
    // Get room summary without loading full room state
    // This is a placeholder implementation
    const room = await this.getRoomAsync(roomId);
    return {
      roomId,
      name: room?.name,
      unreadCount: 0, // Placeholder
      notificationCount: 0, // Placeholder
      lastActivity: new Date(), // Placeholder
    };
  }

  async getTotalRoomCount(): Promise<number> {
    return await this.slidingWindowManager.getTotalRoomCount();
  }
}
