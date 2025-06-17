import { MatrixAgentMessageRequest } from './matrix.agent.dto.message.request.js';

export class MatrixAgentMessageRequestCommunity extends MatrixAgentMessageRequest {
  communityId!: string;
}
