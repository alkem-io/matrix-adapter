
import { MatrixAdminEventUpdateRoomStateForAdminRoomsInput } from '../src/services/matrix-admin/dto/matrix.admin.dto.event.update.room.state.for.admin.rooms';

const messageBody: CmdMatrixAdminEventResetAdminRoomsInput = {
  pattern: 'updateRoomStateForAdminRooms',
  payload: {
    adminEmail: "matrixadmin@alkem.io",
    adminPassword: "change_me_now",
    powerLevel: {
      users_default: 50
    }
  }

}

// {
//   "pattern": "updateRoomStateForAdminRooms",
//   "data": {
//     "adminEmail": "matrixadmin@alkem.io",
//     "adminPassword": "change_me_now",
//     "powerLevel": {
//       "users_default": 50
//     }
//   }
// }

export class CmdMatrixAdminEventResetAdminRoomsInput {
  pattern: string;
  payload: MatrixAdminEventUpdateRoomStateForAdminRoomsInput;
}