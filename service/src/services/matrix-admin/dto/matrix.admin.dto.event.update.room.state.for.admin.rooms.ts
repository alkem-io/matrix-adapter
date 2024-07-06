export class MatrixAdminEventUpdateRoomStateForAdminRoomsInput {
  adminEmail!: string;

  adminPassword!: string;

  powerLevel!: RoomStatePowerLevel;
}

export class RoomStatePowerLevel {
  users_default!: number;
}
