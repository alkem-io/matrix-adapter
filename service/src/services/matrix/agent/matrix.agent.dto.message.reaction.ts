import { MatrixAgentMessageRequest } from './matrix.agent.dto.message.request';

export class MatrixAgentMessageReaction extends MatrixAgentMessageRequest {
  messageID!: string;
}
