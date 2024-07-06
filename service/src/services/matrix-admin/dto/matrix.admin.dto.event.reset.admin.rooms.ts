export class MatrixAdminEventResetAdminRoomsInput {
  adminEmail!: string;

  adminPassword!: string;

  powerLevel!: RoomStatePowerLevel;
}

export class RoomStatePowerLevel {
  users_default!: number;
}
