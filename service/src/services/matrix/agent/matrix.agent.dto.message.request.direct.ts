import { MatrixAgentMessageRequest } from './matrix.agent.dto.message.request.js';

export class MatrixAgentMessageRequestDirect extends MatrixAgentMessageRequest {
  matrixID!: string;
}
