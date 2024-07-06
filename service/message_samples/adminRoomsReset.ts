
import { MatrixAdminEventResetAdminRoomsInput } from '../src/services/matrix-admin/dto/matrix.admin.dto.event.reset.admin.rooms';

const messageBody: CmdMatrixAdminEventResetAdminRoomsInput = {
  pattern: 'adminRoomsReset',
  payload: {
    adminEmail: "matrixadmin@alkem.io",
    adminPassword: "change_me_now",
    powerLevel: {
      users_default: 50
    }
  }

}

// {
//   "pattern": "adminRoomsReset",
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
  payload: MatrixAdminEventResetAdminRoomsInput;
}