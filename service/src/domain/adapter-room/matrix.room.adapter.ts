import { RoomDirectResult } from '@alkemio/matrix-adapter-lib';
import { IMessage, IReaction,RoomResult } from '@alkemio/matrix-adapter-lib';
import { LogContext } from '@common/enums/index';
import { MatrixEntityNotFoundException } from '@common/exceptions/matrix.entity.not.found.exception';
import pkg from '@nestjs/common';
import { MatrixAgent } from '@src/domain/agent/agent/matrix.agent';
import {
  Direction,
  EventType,
  HistoryVisibility,
  ICreateRoomOpts,
  IJoinRoomOpts,
  JoinRule,
  KnownMembership,
  MatrixEvent,
  MsgType,
  Preset,
  RelationType,
  TimelineWindow,
  Visibility,
} from 'matrix-js-sdk';
import {
  ReactionEventContent,
  RoomHistoryVisibilityEventContent,
  RoomJoinRulesEventContent,
  RoomMessageEventContent,
} from 'matrix-js-sdk/lib/types';
import { WINSTON_MODULE_NEST_PROVIDER } from 'nest-winston';

import { MatrixMessageAdapter } from '../adapter-message/matrix.message.adapter';
import { MatrixUserAdapter } from '../adapter-user/matrix.user.adapter';
import { MatrixRoom } from '../room/matrix.room';
import { IRoomOpts } from '../room/matrix.room.options';
import { MatrixRoomMessageReaction } from './dto/matrix.room.dto.message.reaction';
import { MatrixRoomMessageReply } from './dto/matrix.room.dto.message.reply';
import { MatrixRoomMessageRequest } from './dto/matrix.room.dto.message.request';
import { MatrixRoomMessageRequestDirect } from './dto/matrix.room.dto.message.request.direct';
import { MatrixRoomResponseMessage } from './dto/matrix.room.dto.response.message';
const { Inject, Injectable } = pkg;

// Note: before
@Injectable()
export class MatrixRoomAdapter {
  constructor(
    @Inject(WINSTON_MODULE_NEST_PROVIDER)
    private readonly logger: pkg.LoggerService,
    private matrixMessageAdapter: MatrixMessageAdapter,
    private matrixUserAdapter: MatrixUserAdapter
  ) {}

  async getJoinedRooms(agent: MatrixAgent): Promise<string[]> {
    const response = (await agent.matrixClient.getJoinedRooms()) as any as {
      joined_rooms: string[];
    };
    return response.joined_rooms;
  }

  async isUserMemberOfRoom(
    agent: MatrixAgent,
    roomID: string
  ): Promise<boolean> {
    const rooms = await this.getJoinedRooms(agent);
    const roomFound = rooms.find(r => r === roomID);
    if (roomFound) {
      this.logger.verbose?.(
        `[Membership] user (${agent.getUserId()}) is a member of: ${roomID}`,
        LogContext.COMMUNICATION
      );
      return true;
    }
    return false;
  }

  async logJoinedRooms(agent: MatrixAgent) {
    const rooms = await this.getJoinedRooms(agent);
    this.logger.verbose?.(
      `[Membership] user (${agent.getUserId()}) rooms: ${rooms}`,
      LogContext.COMMUNICATION
    );
  }


  public async getDirectRooms(agent: MatrixAgent): Promise<MatrixRoom[]> {
    const rooms: MatrixRoom[] = [];

    // Direct rooms
    const dmRoomMap = agent.getDirectMessageRoomsMap();

    // UPDATED TO USE ASYNC ROOM ACCESS
    for (const matrixUsername of Object.keys(dmRoomMap)) {
      const roomId = dmRoomMap[matrixUsername][0];
      const room = await agent.getRoomOrFail(roomId); // NOW ASYNC

      room.receiverCommunicationsID =
        this.matrixUserAdapter.convertMatrixUsernameToMatrixID(matrixUsername);
      room.isDirect = true;
      rooms.push(room);
    }
    return rooms;
  }

  async initiateMessagingToUser(
    agent: MatrixAgent,
    messageRequest: MatrixRoomMessageRequestDirect
  ): Promise<string> {
    const directRoom = await this.getDirectRoomForMatrixID(
      agent,
      messageRequest.matrixID
    );
    if (directRoom) return directRoom.roomId;

    // Room does not exist, create...
    const targetRoomId = await this.createRoom(
      agent,
      {},
      {
        dmUserId: messageRequest.matrixID,
      }
    );

    await agent.storeDirectMessageRoom(
      agent,
      targetRoomId,
      messageRequest.matrixID
    );

    return targetRoomId;
  }

  /*
    the naming is really confusing
    what we attempt to solve is a race condition
    where two or more DM rooms are created between the two users
    we aim to always resolve the
  */
  getDirectUserMatrixIDForRoomID(
    agent: MatrixAgent,
    matrixRoomId: string
  ): string | undefined {
    // Need to implement caching for performance
    const dmRoomByUserMatrixIDMap = agent.getDirectMessageRoomsMap(
    );
    const dmUserMatrixIDs = Object.keys(dmRoomByUserMatrixIDMap);
    const dmRoom = dmUserMatrixIDs.find(
      userID => dmRoomByUserMatrixIDMap[userID].indexOf(matrixRoomId) !== -1
    );
    return dmRoom;
  }

  public async getDirectRoomForMatrixID(
    agent: MatrixAgent,
    matrixUserId: string
  ): Promise<MatrixRoom | undefined> {
    const matrixUsername =
      this.matrixUserAdapter.convertMatrixIDToUsername(matrixUserId);
    // Need to implement caching for performance
    const dmRoomIds = agent.getDirectMessageRoomsMap()[
      matrixUsername
    ];

    if (!dmRoomIds || !dmRoomIds[0]) {
      return undefined;
    }

    // Have a result
    const targetRoomId = dmRoomIds[0];
    return agent.getRoomOrFail(targetRoomId);
  }

  async sendMessage(
    matrixAgent: MatrixAgent,
    roomId: string,
    messageRequest: MatrixRoomMessageRequest
  ): Promise<string> {
    const content: RoomMessageEventContent = {
      body: messageRequest.text,
      msgtype: MsgType.Text,
    };
    const response = await matrixAgent.sendEvent(
      roomId,
      EventType.RoomMessage,
      content
    );

    return response.event_id;
  }

  async sendReplyToMessage(
    matrixAgent: MatrixAgent,
    roomId: string,
    messageRequest: MatrixRoomMessageReply
  ): Promise<string> {
    const content: RoomMessageEventContent = {
      msgtype: MsgType.Text,
      body: messageRequest.text,
      ['m.relates_to']: {
        rel_type: RelationType.Thread,
        event_id: messageRequest.threadID,
        //is_falling_back: true,
        // when events need to be represented in an unthreaded client, this field makes the event a reply to the thread root event
        // ['m.in_reply_to']: {
        //   event_id: messageRequest.threadID,
        // },
      },
    };

    const response = await matrixAgent.sendEvent(
      roomId,
      EventType.RoomMessage,
      content
    );

    return response.event_id;
  }

  async addReactionOnMessage(
    agent: MatrixAgent,
    roomId: string,
    messageReaction: MatrixRoomMessageReaction
  ): Promise<string> {
    const content: ReactionEventContent = {
      'm.relates_to': {
        rel_type: RelationType.Annotation,
        event_id: messageReaction.messageID,
        key: messageReaction.emoji,

      },
    };

    const response = await agent.matrixClient.sendEvent(
      roomId,
      EventType.Reaction,
      content
    );

    return response.event_id;
  }

  async removeReactionOnMessage(
    matrixAgent: MatrixAgent,
    roomId: string,
    reactionID: string
  ) {
    await matrixAgent.matrixClient.redactEvent(roomId, reactionID);
  }

  // TODO - see if the js sdk supports message aggregation
  editMessage(
    matrixAgent: MatrixAgent,
    roomId: string,
    messageId: string,
    messageRequest: MatrixRoomMessageRequest
  ) {
    this.logger.verbose?.(
      `Editing message: ${messageId} ${matrixAgent} ${roomId} ${messageRequest}`,
      LogContext.COMMUNICATION
    );
    // const newContent: IContent = {
    //   msgtype: MsgType.Text,
    //   body: messageRequest.text,
    // };
    // const content: RoomMessageEventContent = {
    //   msgtype: MsgType.Text,
    //   'm.new_content': newContent,
    //   'm.relates_to': {
    //     rel_type: 'm.replace',
    //     event_id: messageId,
    //   },
    //   body: messageRequest.text,
    // };
    // await matrixAgent.matrixClient.sendMessage(roomId, content);
  }

  async deleteMessage(
    matrixAgent: MatrixAgent,
    roomId: string,
    messageId: string
  ) {
    await matrixAgent.matrixClient.redactEvent(roomId, messageId);
  }

  async createRoom(
    agent: MatrixAgent,
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

    // Set the power levels for the room. Example content of this event below.
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

    const roomResult = await agent.matrixClient.createRoom(createRoomOptions);
    const roomID = roomResult.room_id;
    this.logger.verbose?.(
      `[MatrixRoom] Created new room with id: ${roomID}`,
      LogContext.COMMUNICATION
    );

    if (metadata) {
      await agent.matrixClient.setRoomAccountData(
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

    try {
      const roomJoinOpts: IJoinRoomOpts = {};
      await matrixAgent.matrixClient.joinRoom(roomID, roomJoinOpts);
    } catch (ex: any) {
      this.logger.error?.(
        `[Membership] Exception user joining a room (user: ${matrixAgent.getUserId()}) room: ${roomID}) - ${ex.toString()}`,
        LogContext.COMMUNICATION
      );
    }
  }

  async changeRoomStateJoinRule(
    agent: MatrixAgent,
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
    await agent.matrixClient.sendStateEvent(
      roomID,
      EventType.RoomJoinRules,
      content
    );
  }

  async changeRoomStateHistoryVisibility(
    agent: MatrixAgent,
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
    await agent.matrixClient.sendStateEvent(
      roomID,
      EventType.RoomHistoryVisibility,
      content
    );
  }

  public async getJoinRule(
    agent: MatrixAgent,
    roomID: string
  ): Promise<string> {
    const room = await agent.getRoomOrFail(roomID);
    const roomState = room.currentState;
    return roomState.getJoinRule();
  }

  public async getHistoryVisibility(
    agent: MatrixAgent,
    roomID: string
  ): Promise<HistoryVisibility> {
    const room = await agent.getRoomOrFail(roomID);
    const roomState = room.currentState;
    return roomState.getHistoryVisibility();
  }

  public logRoomMembership(agent: MatrixAgent, roomID: string) {
    const members = this.getMatrixRoomMembers(agent, roomID);
    this.logger.verbose?.(
      `[Membership] Members for room (${roomID}): ${members}`,
      LogContext.COMMUNICATION
    );
  }

  async inviteUserToRoom(
    agentElevated: MatrixAgent,
    roomID: string,
    agentUser: MatrixAgent
  ) {
    const room = await agentElevated.getRoomOrFail(roomID);

    // not very well documented but we can validate whether the user has membership like this
    // seen in https://github.com/matrix-org/matrix-js-sdk/blob/3c36be9839091bf63a4850f4babed0c976d48c0e/src/models/room-member.ts#L29
    const userId = agentUser.getUserId();

    if (room.hasMembershipState(userId, KnownMembership.Join)) {
      return;
    }
    if (room.hasMembershipState(userId, KnownMembership.Invite)) {
      await agentUser.matrixClient.joinRoom(room.roomId);
      return;
    }

    await agentElevated.matrixClient.invite(roomID, userId);
    this.logger.verbose?.(
      `invited user to room: ${userId} - ${roomID}`,
      LogContext.COMMUNICATION
    );
  }

  async removeUserFromRoom(
    agentElevated: MatrixAgent,
    roomID: string,
    agentUser: MatrixAgent
  ) {
    const matrixUserID = agentUser.getUserId();
    try {
      this.logger.verbose?.(
        `[Membership] User (${matrixUserID}) being removed from room: ${roomID}`,
        LogContext.COMMUNICATION
      );

      // force the user to leave
      // https://github.com/matrix-org/matrix-js-sdk/blob/b2d83c1f80b53ae3ca396ac102edf27bfbbd7015/src/client.ts#L4354
      await agentElevated.matrixClient.kick(roomID, matrixUserID);
    } catch (error) {
      this.logger.verbose?.(
        `Unable to remove user (${matrixUserID}) from rooms (${roomID}): ${error}`,
        LogContext.COMMUNICATION
      );
    }
  }

  async getAllRoomEvents(agent: MatrixAgent, matrixRoom: MatrixRoom) {
    // do NOT use the deprecated room.timeline property
    const timeline = matrixRoom.getLiveTimeline();
    const loadedEvents = timeline.getEvents();
    const lastKnownEvent = loadedEvents[loadedEvents.length - 1];
    // got the idea from - components/structures/TimelinePanel.tsx in matrix-react-sdk
    const timelineWindow = new TimelineWindow(
      agent.matrixClient,
      timeline.getTimelineSet()
    );

    // need to set an anchor on the last known event and load from there
    if (lastKnownEvent) {
      await timelineWindow.load(lastKnownEvent.getId(), 1000);
      // attempt to paginate while we have outstanding messages
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
    agent: MatrixAgent,
    matrixRoom: MatrixRoom,
    receiverMatrixID: string
  ): Promise<RoomDirectResult> {
    const roomResult = new RoomDirectResult();
    roomResult.id = matrixRoom.roomId;
    roomResult.messages = await this.getMatrixRoomTimelineAsMessages(
      agent,
      matrixRoom
    );
    roomResult.receiverID = receiverMatrixID;

    return roomResult;
  }

  async convertMatrixRoomToCommunityRoom(
    agent: MatrixAgent,
    matrixRoom: MatrixRoom
  ): Promise<RoomResult> {
    const roomResult = new RoomResult();
    roomResult.id = matrixRoom.roomId;
    roomResult.messages = await this.getMatrixRoomTimelineAsMessages(
      agent,
      matrixRoom
    );
    roomResult.displayName = matrixRoom.name;

    return roomResult;
  }

  async getMatrixRoomTimelineAsMessages(
    agent: MatrixAgent,
    matrixRoom: MatrixRoom
  ): Promise<IMessage[]> {
    this.logger.verbose?.(
      `[MatrixRoom] Obtaining messages on room: ${matrixRoom.name}`,
      LogContext.COMMUNICATION
    );

    const timelineEvents = await this.getAllRoomEvents(agent, matrixRoom);
    if (timelineEvents) {
      return this.convertMatrixTimelineToMessages(timelineEvents);
    }
    return [];
  }

  async getMatrixRoomTimelineReactions(
    agent: MatrixAgent,
    matrixRoom: MatrixRoom
  ): Promise<IReaction[]> {
    this.logger.verbose?.(
      `[MatrixRoom] Obtaining messages on room: ${matrixRoom.name}`,
      LogContext.COMMUNICATION
    );

    const timelineEvents = await this.getAllRoomEvents(agent, matrixRoom);
    if (timelineEvents) {
      return this.convertMatrixTimelineToReactions(timelineEvents);
    }
    return [];
  }

  convertMatrixTimelineToMessages(
    timeline: MatrixRoomResponseMessage[]
  ): IMessage[] {
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

  convertMatrixTimelineToReactions(
    timeline: MatrixEvent[]
  ): IReaction[] {
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

  public async getMatrixRoomMembers(
    agent: MatrixAgent,
    matrixRoomID: string
  ): Promise<string[]> {
    this.logger.verbose?.(
      `[MatrixRoom] Obtaining members on room: ${matrixRoomID}`,
      LogContext.COMMUNICATION
    );
    const room = await agent.getRoomOrFail(matrixRoomID);
    const roomMembers = room.getMembers();
    const usersIDs: string[] = [];
    for (const roomMember of roomMembers) {
      // skip the elevated admin
      if (roomMember.userId === agent.getUserId()) continue;

      if (roomMember.membership === KnownMembership.Join) {
        usersIDs.push(roomMember.userId);
      }
    }
    return usersIDs;
  }
}
