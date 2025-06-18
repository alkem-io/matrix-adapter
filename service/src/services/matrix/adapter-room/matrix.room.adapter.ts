import { RoomDirectResult } from '@alkemio/matrix-adapter-lib';
import { RoomResult, IMessage, IReaction } from '@alkemio/matrix-adapter-lib';
import { LogContext } from '@common/enums/index';
import { MatrixEntityNotFoundException } from '@common/exceptions/matrix.entity.not.found.exception';
import pkg  from '@nestjs/common';
const { Inject, Injectable } = pkg;
import {
  Direction,
  EventType,
  HistoryVisibility,
  IContent,
  ICreateRoomOpts,
  IJoinRoomOpts,
  JoinRule,
  MatrixClient,
  MatrixEvent,
  Preset,
  TimelineWindow,
  Visibility,
} from 'matrix-js-sdk';
import { WINSTON_MODULE_NEST_PROVIDER } from 'nest-winston';
import { MatrixMessageAdapter } from '../adapter-message/matrix.message.adapter';
import { MatrixRoom } from './matrix.room';
import { MatrixRoomResponseMessage } from './matrix.room.dto.response.message';
import {
  RoomHistoryVisibilityEventContent,
  RoomJoinRulesEventContent,
} from 'matrix-js-sdk/lib/types';
import { MatrixAgent } from '../agent/matrix.agent';
import { IRoomOpts } from './matrix.room.dto.options';

@Injectable()
export class MatrixRoomAdapter {
  constructor(
    @Inject(WINSTON_MODULE_NEST_PROVIDER)
    private readonly logger: pkg.LoggerService,
    private matrixMessageAdapter: MatrixMessageAdapter
  ) {}

  async storeDirectMessageRoom(
    matrixClient: MatrixClient,
    roomId: string,
    userId: string
  ) {
    // NOT OPTIMIZED - needs caching
    const dmRooms = this.getDirectMessageRoomsMap(matrixClient);

    dmRooms[userId] = [roomId];
    await matrixClient.setAccountData(EventType.Direct, dmRooms);
  }

  // there could be more than one dm room per user
  getDirectMessageRoomsMap(
    matrixClient: MatrixClient
  ): Record<string, string[]> {
    const mDirectEvent = matrixClient.getAccountData(EventType.Direct);
    // todo: tidy up this logic
    const eventContent = mDirectEvent
      ? mDirectEvent.getContent<IContent>()
      : {};

    const userId = matrixClient.getUserId();
    if (!userId) {
      throw new MatrixEntityNotFoundException(
        `Unable to locate userId for client: ${matrixClient}`,
        LogContext.MATRIX
      );
    }

    // there is a bug in the sdk
    const selfDMs = eventContent[userId];
    if (selfDMs && selfDMs.length) {
      // it seems that two users can have multiple DM rooms between them and only one needs to be active
      // they have fixed the issue inside the react-sdk instead of the js-sdk...
    }

    return eventContent;
  }

  async createRoom(
    matrixClient: MatrixClient,
    createRoomOptions: ICreateRoomOpts,
    alkemioOptions: IRoomOpts
  ): Promise<string> {
    const { dmUserId, metadata } = alkemioOptions;

    // all rooms will by default be public
    const defaultPreset = Preset.PublicChat;
    createRoomOptions.preset = createRoomOptions.preset || defaultPreset;
    // all rooms will by default be public and visible - need to revise this
    // once the Synapse server is community accessible
    createRoomOptions.visibility =
      createRoomOptions.visibility || Visibility.Public;

    if (dmUserId && createRoomOptions.invite === undefined) {
      createRoomOptions.invite = [dmUserId];
    }
    if (dmUserId && createRoomOptions.is_direct === undefined) {
      createRoomOptions.is_direct = true;
    }

    // Set the pwer levels for the room. Example content of this event below.
    // [m.room.power_levels] - [$ltIxj3IMlH1Plkm-1dOmahrrO3brsPNAvLiUM43liWI] - content: {
    //   "users":{"@matrixadmin4=alkem.io:alkemio.matrix.host":100},"users_default":0,"events":
    //   {"m.room.name":50,"m.room.power_levels":100,"m.room.history_visibility":100,"m.room.canonical_alias":50,
    //     "m.room.avatar":50,"m.room.tombstone":100,"m.room.server_acl":100,"m.room.encryption":100},
    //     "events_default":0,"state_default":50,"ban":50,"kick":50,"redact":50,"invite":50,"historical":100} - {}
    // See: https://spec.matrix.org/v1.3/client-server-api/ + search on m.room.power_levels
    // Explicitly set that new users have full admin power
    createRoomOptions.power_level_content_override = {
      users_default: 50,
      redact: 0,
    };

    const roomResult = await matrixClient.createRoom(createRoomOptions);
    const roomID = roomResult.room_id;
    this.logger.verbose?.(
      `[MatrixRoom] Created new room with id: ${roomID}`,
      LogContext.COMMUNICATION
    );

    if (metadata) {
      await matrixClient.setRoomAccountData(
        roomID,
        'alkemio.metadata',
        metadata
      );
    }

    return roomID;
  }

  async joinRoomSafe(matrixAgent: MatrixAgent, roomID: string): Promise<void> {
    if (roomID === '')
      throw new MatrixEntityNotFoundException(
        'No room ID specified',
        LogContext.COMMUNICATION
      );

    const matrixClient = matrixAgent.matrixClient;
    try {
      const roomJoinOpts: IJoinRoomOpts = {};
      await matrixClient.joinRoom(roomID, roomJoinOpts);
    } catch (ex: any) {
      this.logger.error?.(
        `[Membership] Exception user joining a room (user: ${matrixClient.getUserId()}) room: ${roomID}) - ${ex.toString()}`,
        LogContext.COMMUNICATION
      );
    }
  }

  async changeRoomStateJoinRule(
    matrixClient: MatrixClient,
    roomID: string,
    state: JoinRule
  ): Promise<void> {
    if (roomID === '')
      throw new MatrixEntityNotFoundException(
        'No room ID specified',
        LogContext.COMMUNICATION
      );

    // This was 'm.room.join_rules'
    const content: RoomJoinRulesEventContent = {
      join_rule: state,
    };
    await matrixClient.sendStateEvent(roomID, EventType.RoomJoinRules, content);
  }

  async changeRoomStateHistoryVisibility(
    matrixClient: MatrixClient,
    roomID: string,
    state: HistoryVisibility
  ): Promise<void> {
    if (roomID === '')
      throw new MatrixEntityNotFoundException(
        'No room ID specified',
        LogContext.COMMUNICATION
      );

    // This was 'm.room.join_rules'
    const content: RoomHistoryVisibilityEventContent = {
      history_visibility: state,
    };
    await matrixClient.sendStateEvent(
      roomID,
      EventType.RoomHistoryVisibility,
      content
    );
  }

  public async getJoinRule(
    adminMatrixClient: MatrixClient,
    roomID: string
  ): Promise<string> {
    const room = await this.getMatrixRoom(adminMatrixClient, roomID);
    const roomState = room.currentState;
    return roomState.getJoinRule();
  }

  public async getHistoryVisibility(
    adminMatrixClient: MatrixClient,
    roomID: string
  ): Promise<HistoryVisibility> {
    const room = await this.getMatrixRoom(adminMatrixClient, roomID);
    const roomState = room.currentState;
    return roomState.getHistoryVisibility();
  }

  async getMatrixRoom(
    adminMatrixClient: MatrixClient,
    roomID: string
  ): Promise<MatrixRoom> {
    if (roomID === '')
      throw new MatrixEntityNotFoundException(
        'No room ID specified',
        LogContext.COMMUNICATION
      );
    // need to cache those
    const room = await adminMatrixClient.getRoom(roomID);
    if (!room) {
      throw new MatrixEntityNotFoundException(
        `Unable to locate room: ${roomID}`,
        LogContext.COMMUNICATION
      );
    }
    return room;
  }

  async logRoomMembership(adminMatrixClient: MatrixClient, roomID: string) {
    const members = await this.getMatrixRoomMembers(adminMatrixClient, roomID);
    this.logger.verbose?.(
      `[Membership] Members for room (${roomID}): ${members}`,
      LogContext.COMMUNICATION
    );
  }
  async inviteUserToRoom(
    adminMatrixClient: MatrixClient,
    roomID: string,
    matrixClient: MatrixClient
  ) {
    const room = await this.getMatrixRoom(adminMatrixClient, roomID);

    // not very well documented but we can validate whether the user has membership like this
    // seen in https://github.com/matrix-org/matrix-js-sdk/blob/3c36be9839091bf63a4850f4babed0c976d48c0e/src/models/room-member.ts#L29
    const userId = matrixClient.getUserId();
    if (!userId) {
      throw new MatrixEntityNotFoundException(
        `Unable to locate userId for client: ${matrixClient}`,
        LogContext.MATRIX
      );
    }
    if (room.hasMembershipState(userId, 'join')) {
      return;
    }
    if (room.hasMembershipState(userId, 'invite')) {
      await matrixClient.joinRoom(room.roomId);
      return;
    }

    await adminMatrixClient.invite(roomID, userId);
    this.logger.verbose?.(
      `invited user to room: ${userId} - ${roomID}`,
      LogContext.COMMUNICATION
    );
  }

  async removeUserFromRoom(
    adminMatrixClient: MatrixClient,
    roomID: string,
    matrixClient: MatrixClient
  ) {
    const matrixUserID = matrixClient.getUserId();
    if (!matrixUserID) {
      throw new MatrixEntityNotFoundException(
        `Unable to locate userId for client: ${matrixClient}`,
        LogContext.MATRIX
      );
    }
    try {
      this.logger.verbose?.(
        `[Membership] User (${matrixUserID}) being removed from room: ${roomID}`,
        LogContext.COMMUNICATION
      );

      // force the user to leave
      // https://github.com/matrix-org/matrix-js-sdk/blob/b2d83c1f80b53ae3ca396ac102edf27bfbbd7015/src/client.ts#L4354
      await adminMatrixClient.kick(roomID, matrixUserID);
    } catch (error) {
      this.logger.verbose?.(
        `Unable to remove user (${matrixUserID}) from rooms (${roomID}): ${error}`,
        LogContext.COMMUNICATION
      );
    }
  }

  async getAllRoomEvents(client: MatrixClient, matrixRoom: MatrixRoom) {
    // do NOT use the deprecated room.timeline property
    const timeline = matrixRoom.getLiveTimeline();
    const loadedEvents = timeline.getEvents();
    const lastKnownEvent = loadedEvents[loadedEvents.length - 1];
    // got the idea from - components/structures/TimelinePanel.tsx in matrix-react-sdk
    const timelineWindow = new TimelineWindow(
      client,
      timeline.getTimelineSet()
    );

    // need to set an anchor on the last known event and load from there
    if (lastKnownEvent) {
      await timelineWindow.load(lastKnownEvent.getId(), 1000);
      // atempt to paginate while we have outstanding messages
      while (timelineWindow.canPaginate(Direction.Backward)) {
        // do the actual event loading in memory
        await timelineWindow.paginate(Direction.Backward, 1000);
      }
    }

    const events = timelineWindow.getEvents();
    let i = 1;
    for (const event of events) {
      this.logger.verbose?.(
        `Event [${i}] - [${event.getType()}] - [${
          event.event.event_id
        }] - content: ${JSON.stringify(event.getContent())}`,
        LogContext.COMMUNICATION
      );
      i++;
    }

    return events;
  }

  async convertMatrixRoomToDirectRoom(
    matrixClient: MatrixClient,
    matrixRoom: MatrixRoom,
    receiverMatrixID: string
  ): Promise<RoomDirectResult> {
    const roomResult = new RoomDirectResult();
    roomResult.id = matrixRoom.roomId;
    roomResult.messages = await this.getMatrixRoomTimelineAsMessages(
      matrixClient,
      matrixRoom
    );
    roomResult.receiverID = receiverMatrixID;

    return roomResult;
  }

  async convertMatrixRoomToCommunityRoom(
    matrixClient: MatrixClient,
    matrixRoom: MatrixRoom
  ): Promise<RoomResult> {
    const roomResult = new RoomResult();
    roomResult.id = matrixRoom.roomId;
    roomResult.messages = await this.getMatrixRoomTimelineAsMessages(
      matrixClient,
      matrixRoom
    );
    roomResult.displayName = matrixRoom.name;

    return roomResult;
  }

  async getMatrixRoomTimelineAsMessages(
    matrixClient: MatrixClient,
    matrixRoom: MatrixRoom
  ): Promise<IMessage[]> {
    this.logger.verbose?.(
      `[MatrixRoom] Obtaining messages on room: ${matrixRoom.name}`,
      LogContext.COMMUNICATION
    );

    const timelineEvents = await this.getAllRoomEvents(
      matrixClient,
      matrixRoom
    );
    if (timelineEvents) {
      return await this.convertMatrixTimelineToMessages(timelineEvents);
    }
    return [];
  }

  async getMatrixRoomTimelineReactions(
    matrixClient: MatrixClient,
    matrixRoom: MatrixRoom
  ): Promise<IReaction[]> {
    this.logger.verbose?.(
      `[MatrixRoom] Obtaining messages on room: ${matrixRoom.name}`,
      LogContext.COMMUNICATION
    );

    const timelineEvents = await this.getAllRoomEvents(
      matrixClient,
      matrixRoom
    );
    if (timelineEvents) {
      return await this.convertMatrixTimelineToReactions(timelineEvents);
    }
    return [];
  }

  async convertMatrixTimelineToMessages(
    timeline: MatrixRoomResponseMessage[]
  ): Promise<IMessage[]> {
    const messages: IMessage[] = [];
    const reactionsMap = new Map<string, IReaction[]>();

    for (const timelineEvent of timeline) {
      if (this.matrixMessageAdapter.isEventToIgnore(timelineEvent)) continue;
      if (this.matrixMessageAdapter.isEventMessage(timelineEvent)) {
        const message =
          this.matrixMessageAdapter.convertFromMatrixMessage(timelineEvent);

        messages.push(message);
      }
      if (this.matrixMessageAdapter.isEventReaction(timelineEvent)) {
        const reaction =
          this.matrixMessageAdapter.convertFromMatrixReaction(timelineEvent);

        const messageReactions = reactionsMap.get(reaction.messageId) ?? [];
        if (messageReactions) {
          messageReactions.push(reaction);
        }
        reactionsMap.set(reaction.messageId, messageReactions);
      }
    }
    for (const message of messages) {
      message.reactions = reactionsMap.get(message.id) ?? [];
    }

    this.logger.verbose?.(
      `[MatrixRoom] Timeline converted: ${timeline.length} events ==> ${messages.length} messages`,
      LogContext.COMMUNICATION
    );
    return messages;
  }

  async convertMatrixTimelineToReactions(
    timeline: MatrixEvent[]
  ): Promise<IReaction[]> {
    const reactions: IReaction[] = [];

    for (const timelineEvent of timeline) {
      if (this.matrixMessageAdapter.isEventToIgnore(timelineEvent)) continue;
      if (this.matrixMessageAdapter.isEventReaction(timelineEvent)) {
        const reaction =
          this.matrixMessageAdapter.convertFromMatrixReaction(timelineEvent);

        reactions.push(reaction);
      }
    }

    this.logger.verbose?.(
      `[MatrixRoom] Timeline converted: ${timeline.length} events ==> ${reactions.length} reactions`,
      LogContext.COMMUNICATION
    );
    return reactions;
  }

  async getMatrixRoomMembers(
    adminMatrixClient: MatrixClient,
    matrixRoomID: string
  ): Promise<string[]> {
    this.logger.verbose?.(
      `[MatrixRoom] Obtaining members on room: ${matrixRoomID}`,
      LogContext.COMMUNICATION
    );
    const room = await this.getMatrixRoom(adminMatrixClient, matrixRoomID);
    const roomMembers = room.getMembers();
    const usersIDs: string[] = [];
    for (const roomMember of roomMembers) {
      // skip the elevated admin
      if (roomMember.userId === adminMatrixClient.getUserId()) continue;

      if (roomMember.membership === 'join') {
        usersIDs.push(roomMember.userId);
      }
    }
    return usersIDs;
  }
}
