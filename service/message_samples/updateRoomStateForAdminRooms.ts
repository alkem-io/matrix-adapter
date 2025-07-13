import { MatrixAdminEventUpdateRoomStateForAdminRoomsInput } from '../src/domain/matrix-admin/rooms/dto/matrix.admin.roomsdto.event.update.room.state.for.admin.rooms.js';

const messageBody: CmdMatrixAdminEventResetAdminRoomsInput = {
  pattern: 'updateRoomStateForAdminRooms',
  payload: {
    adminEmail: "matrixadmin@alkem.io",
    adminPassword: "change_me_now",
    powerLevel: {
      users_default: 50,
      redact: 0
    }
  }
}

{
  "pattern": "updateRoomStateForAdminRooms",
  "data": {
    "adminEmail": "matrixadmin@alkem.io",
    "adminPassword": "change_me_now",
    "powerLevel": {
      "users_default": 50,
      "redact": 0
    }
  }
}

export class CmdMatrixAdminEventResetAdminRoomsInput {
  pattern: string;
  payload: MatrixAdminEventUpdateRoomStateForAdminRoomsInput;
}