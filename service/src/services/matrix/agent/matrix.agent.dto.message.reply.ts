import { MatrixAgentMessageRequest } from './matrix.agent.dto.message.request';

export class MatrixAgentMessageReply extends MatrixAgentMessageRequest {
  threadID!: string;
}
