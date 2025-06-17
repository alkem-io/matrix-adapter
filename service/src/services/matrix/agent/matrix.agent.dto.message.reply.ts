import { MatrixAgentMessageRequest } from './matrix.agent.dto.message.request.js';

export class MatrixAgentMessageReply extends MatrixAgentMessageRequest {
  threadID!: string;
}
