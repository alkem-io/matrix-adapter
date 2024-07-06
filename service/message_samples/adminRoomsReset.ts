
import { MatrixAdminEventResetAdminRoomsInput } from '../src/services/matrix-admin/dto/matrix.admin.dto.event.reset.admin.rooms';

const messageBody: CmdMatrixAdminEventResetAdminRoomsInput = {
  cmd: 'adminRoomsReset',
  payload: {
    adminEmail: "matrixadmin@alkem.io",
    adminPassword: "change-me-now",
    powerLevel: {
      users_default: 50
    }
  }

}

// {
//   "cmd": "adminRoomsReset",
//   "data": {
//     "adminEmail": "matrixadmin@alkem.io",
//     "adminPassword": "change-me-now",
//     "powerLevel": {
//       "users_default": 50
//     }
//   }
// }

export class CmdMatrixAdminEventResetAdminRoomsInput {
  cmd: string;
  payload: MatrixAdminEventResetAdminRoomsInput;
}