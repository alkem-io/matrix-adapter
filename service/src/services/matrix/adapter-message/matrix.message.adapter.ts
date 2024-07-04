import { IMessage, IReaction } from '@alkemio/matrix-adapter-lib';
import { LogContext } from '@common/enums';
import { Inject, Injectable, LoggerService } from '@nestjs/common';
import { WINSTON_MODULE_NEST_PROVIDER } from 'nest-winston';
import { MatrixRoomResponseMessage } from '../adapter-room/matrix.room.dto.response.message';
import { MatrixEntityNotFoundException } from '@src/common/exceptions';
import { EventType } from 'matrix-js-sdk';

@Injectable()
export class MatrixMessageAdapter {
  readonly ALLOWED_EVENT_TYPES = [EventType.RoomMessage, EventType.Reaction];

  constructor(
    @Inject(WINSTON_MODULE_NEST_PROVIDER)
    private readonly logger: LoggerService
  ) {}

  convertFromMatrixMessage(message: MatrixRoomResponseMessage): IMessage {
    const { event, sender, threadRootId } = message;

    // need to use getContent - should be able to resolve the edited value if any
    const content = message.getContent();

    // these are used to detect whether a message is a replacement one
    // const isRelation = message.isRelation('m.replace');
    // const mRelatesTo = message.getWireContent()['m.relates_to'];

    if (!sender) {
      throw new MatrixEntityNotFoundException(
        `Unable to locate userId for sender: ${sender}`,
        LogContext.MATRIX
      );
    }
    const sendingUserID = sender.userId;

    return {
      message: content.body,
      sender: sendingUserID,
      timestamp: event.origin_server_ts || 0,
      id: event.event_id || '',
      reactions: [],
      threadID: threadRootId,
    };
  }

  convertFromMatrixReaction(reaction: MatrixRoomResponseMessage): IReaction {
    const { event, sender } = reaction;

    // need to use getContent - should be able to resolve the edited value if any
    const content = reaction.getContent();

    // these are used to detect whether a message is a replacement one
    // const isRelation = message.isRelation('m.replace');
    // const mRelatesTo = message.getWireContent()['m.relates_to'];

    if (!sender) {
      throw new MatrixEntityNotFoundException(
        `Unable to locate userId for sender: ${sender}`,
        LogContext.MATRIX
      );
    }
    const sendingUserID = sender.userId;

    return {
      emoji: content['m.relates_to']?.key || '',
      messageId: content['m.relates_to']?.event_id || '',
      sender: sendingUserID,
      timestamp: event.origin_server_ts || 0,
      id: event.event_id || '',
    };
  }

  isEventMessage(timelineEvent: MatrixRoomResponseMessage): boolean {
    const event = timelineEvent.event;

    if (event.type == EventType.RoomMessage) return true;
    return false;
  }

  isEventReaction(timelineEvent: MatrixRoomResponseMessage): boolean {
    const event = timelineEvent.event;

    if (event.type == EventType.Reaction) return true;
    return false;
  }

  isEventToIgnore(message: MatrixRoomResponseMessage): boolean {
    const event = message.event;

    if (
      event.type &&
      this.ALLOWED_EVENT_TYPES.every(type => event.type !== type)
    ) {
      this.logger.verbose?.(
        `[Timeline] Ignoring event of type: ${event.type} as it is not one of '${this.ALLOWED_EVENT_TYPES}' types `,
        LogContext.COMMUNICATION
      );
      return true;
    }

    const content = message.getContent();
    if (!content) {
      this.logger.verbose?.(
        `[Timeline] Ignoring event with no content: ${event.type} - id: ${event.event_id}`,
        LogContext.COMMUNICATION
      );
      return true;
    }

    if (event.type === EventType.RoomMessage && !content.body) {
      this.logger.verbose?.(
        `[Timeline] Ignoring mesage event with no content body: ${event.type} - id: ${event.event_id}`,
        LogContext.COMMUNICATION
      );
      return true;
    }

    if (event.type === EventType.Reaction && !content['m.relates_to']) {
      this.logger.verbose?.(
        `[Timeline] Ignoring reaction event with no realates_to property: ${event.type} - id: ${event.event_id}`,
        LogContext.COMMUNICATION
      );
      return true;
    }
    // Want to ignore acknowledgements
    // if (event.event_id?.indexOf(event.room_id || '') !== -1) {
    //   this.logger.verbose?.(
    //     `[MessageAction] Identified as temporary: ${event.type} - ${event.event_id}`,
    //     LogContext.COMMUNICATION
    //   );
    //   //return true;
    // }
    this.logger.verbose?.(
      `[Timeline] Processing event: ${event.type} - ${event.event_id}`,
      LogContext.COMMUNICATION
    );
    return false;
  }
}
