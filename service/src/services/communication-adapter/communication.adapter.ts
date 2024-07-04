import { ConfigurationTypes, LogContext } from '@common/enums';
import {
  MatrixEntityNotFoundException,
  NotEnabledException,
} from '@common/exceptions';
import { Inject, Injectable, LoggerService } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import { MatrixAgentPool } from '@services/matrix/agent-pool/matrix.agent.pool';
import { HistoryVisibility, JoinRule, MatrixClient } from 'matrix-js-sdk';
import { WINSTON_MODULE_NEST_PROVIDER } from 'nest-winston';
import { MatrixRoomAdapter } from '@services/matrix/adapter-room/matrix.room.adapter';
import { MatrixUserAdapter } from '@services/matrix/adapter-user/matrix.user.adapter';
import { IOperationalMatrixUser } from '@services/matrix/adapter-user/matrix.user.interface';
import { MatrixAgent } from '@services/matrix/agent/matrix.agent';
import { MatrixAgentService } from '@services/matrix/agent/matrix.agent.service';
import { MatrixUserManagementService } from '@services/matrix/management/matrix.user.management.service';
import { CommunicationEditMessageInput } from './dto/communication.dto.message.edit';
import {
  IMessage,
  RoomSendMessagePayload,
  RoomSendMessageReplyPayload,
  RoomAddMessageReactionPayload,
  RoomRemoveMessageReactionPayload,
  UpdateRoomStatePayload,
  RoomDetailsResponsePayload,
} from '@alkemio/matrix-adapter-lib';
import { RoomResult } from '@alkemio/matrix-adapter-lib';
import { RoomDirectResult } from '@alkemio/matrix-adapter-lib';
import { RoomDeleteMessagePayload } from '@alkemio/matrix-adapter-lib';
import { SendMessageToUserPayload } from '@alkemio/matrix-adapter-lib';
import { IReaction } from '@alkemio/matrix-adapter-lib';
import { sleep } from 'matrix-js-sdk/lib/utils';

@Injectable()
export class CommunicationAdapter {
  private adminUser!: IOperationalMatrixUser;
  private matrixElevatedAgent!: MatrixAgent; // elevated as created with an admin account
  private adminEmail!: string;
  private adminCommunicationsID!: string;
  private adminPassword!: string;
  private enabled = false;

  constructor(
    @Inject(WINSTON_MODULE_NEST_PROVIDER)
    private readonly logger: LoggerService,
    private matrixAgentService: MatrixAgentService,
    private matrixAgentPool: MatrixAgentPool,
    private configService: ConfigService,
    private matrixUserManagementService: MatrixUserManagementService,
    private matrixUserAdapter: MatrixUserAdapter,
    private matrixRoomAdapter: MatrixRoomAdapter
  ) {
    this.adminEmail = this.configService.get(
      ConfigurationTypes.MATRIX
    )?.admin?.username;
    this.adminPassword = this.configService.get(
      ConfigurationTypes.MATRIX
    )?.admin?.password;

    this.adminCommunicationsID = this.matrixUserAdapter.convertEmailToMatrixID(
      this.adminEmail
    );

    // need both to be true
    this.enabled = true;
  }

  public async initiateMatrixConnection() {
    await this.logServerVersion();
    this.logger.verbose?.(
      `Matrix admin identifier: ${this.adminCommunicationsID}`,
      LogContext.BOOTSTRAP
    );
    await this.getMatrixManagementAgentElevated();
  }

  async getCommunityRoom(
    roomId: string,
    withState = false
  ): Promise<RoomResult> {
    // If not enabled just return an empty room
    if (!this.enabled) {
      const result: RoomResult = {
        id: 'communications-not-enabled',
        messages: [],
        displayName: '',
        members: [],
      };
      return result;
    }
    const matrixAgentElevated = await this.getMatrixManagementAgentElevated();
    const matrixClient = matrixAgentElevated.matrixClient;
    const adminUser = this.getUserIdFromMatrixClient(matrixClient);

    await this.addUserToRoom(roomId, adminUser);
    this.logger.verbose?.(`User added to ${roomId} room`);

    const matrixRoom = await this.matrixAgentService.getRoom(
      matrixAgentElevated,
      roomId
    );
    const result =
      await this.matrixRoomAdapter.convertMatrixRoomToCommunityRoom(
        matrixAgentElevated.matrixClient,
        matrixRoom
      );
    if (withState) {
      result.joinRule = await this.getRoomStateJoinRule(
        roomId,
        matrixAgentElevated
      );
      result.historyVisibility = await this.getRoomStateHistoryVisibility(
        roomId,
        matrixAgentElevated
      );
    }
    return result;
  }

  async sendMessage(
    sendMessageData: RoomSendMessagePayload
  ): Promise<IMessage> {
    // Todo: replace with proper data validation
    const message = sendMessageData.message;

    const senderCommunicationID = sendMessageData.senderID;
    const matrixAgent = await this.acquireMatrixAgent(senderCommunicationID);

    await this.matrixUserAdapter.verifyRoomMembershipOrFail(
      matrixAgent.matrixClient,
      sendMessageData.roomID
    );
    this.logger.verbose?.(
      `[Message sending] Sending message to room: ${sendMessageData.roomID}`,
      LogContext.COMMUNICATION
    );
    let messageId = '';
    try {
      messageId = await this.matrixAgentService.sendMessage(
        matrixAgent,
        sendMessageData.roomID,
        {
          text: sendMessageData.message,
        }
      );
    } catch (error: any) {
      this.logger.error(
        `[Message sending] Unable to send message for user (${senderCommunicationID}): ${error}`,
        LogContext.COMMUNICATION
      );
      throw error;
    }
    this.logger.verbose?.(
      `...message sent to room: ${sendMessageData.roomID}`,
      LogContext.COMMUNICATION
    );

    // Create the 'equivalent' message. Note that this can have a very minor timestamp offset
    // from the actual message.
    const timestamp = new Date().getTime();
    return {
      id: messageId,
      message: message,
      sender: sendMessageData.senderID,
      timestamp: timestamp,
      reactions: [],
    };
  }

  async sendMessageReply(
    sendMessageData: RoomSendMessageReplyPayload
  ): Promise<IMessage> {
    // Todo: replace with proper data validation
    const message = sendMessageData.message;

    const senderCommunicationID = sendMessageData.senderID;
    const matrixAgent = await this.acquireMatrixAgent(senderCommunicationID);

    await this.matrixUserAdapter.verifyRoomMembershipOrFail(
      matrixAgent.matrixClient,
      sendMessageData.roomID
    );
    this.logger.verbose?.(
      `[Message sending] Sending message to room: ${sendMessageData.roomID}`,
      LogContext.COMMUNICATION
    );
    let messageId = '';
    try {
      messageId = await this.matrixAgentService.sendReplyToMessage(
        matrixAgent,
        sendMessageData.roomID,
        {
          text: sendMessageData.message,
          threadID: sendMessageData.threadID,
        }
      );
    } catch (error: any) {
      this.logger.error(
        `[Message sending] Unable to send reply on message for user (${senderCommunicationID}): ${error}`,
        LogContext.COMMUNICATION
      );
      throw error;
    }
    this.logger.verbose?.(
      `...message sent to room: ${sendMessageData.roomID}`,
      LogContext.COMMUNICATION
    );

    // Create the 'equivalent' message. Note that this can have a very minor timestamp offset
    // from the actual message.
    const timestamp = new Date().getTime();
    return {
      id: messageId,
      message: message,
      sender: sendMessageData.senderID,
      timestamp: timestamp,
      reactions: [],
      threadID: sendMessageData.threadID,
    };
  }

  async addReactionToMessage(
    addReactionData: RoomAddMessageReactionPayload
  ): Promise<IReaction> {
    // Todo: replace with proper data validation
    const emoji = addReactionData.emoji;

    const senderCommunicationID = addReactionData.senderID;
    const matrixAgent = await this.acquireMatrixAgent(senderCommunicationID);

    await this.matrixUserAdapter.verifyRoomMembershipOrFail(
      matrixAgent.matrixClient,
      addReactionData.roomID
    );
    this.logger.verbose?.(
      `[Adding reaction] Adding reaction to message in room: ${addReactionData.roomID}`,
      LogContext.COMMUNICATION
    );
    let reactionId = '';
    try {
      reactionId = await this.matrixAgentService.addReactionOnMessage(
        matrixAgent,
        addReactionData.roomID,
        {
          emoji: addReactionData.emoji,
          messageID: addReactionData.messageID,
        }
      );
    } catch (error: any) {
      this.logger.error(
        `[Adding reaction] Unable to add reaction to message for user (${senderCommunicationID}): ${error}`,
        LogContext.COMMUNICATION
      );
      throw error;
    }
    this.logger.verbose?.(
      `...reaction added to message in room: ${addReactionData.roomID}`,
      LogContext.COMMUNICATION
    );

    // Create the 'equivalent' message. Note that this can have a very minor timestamp offset
    // from the actual message.
    const timestamp = new Date().getTime();
    return {
      id: reactionId,
      messageId: addReactionData.messageID,
      emoji: emoji,
      sender: addReactionData.senderID,
      timestamp: timestamp,
    };
  }

  async removeReactionToMessage(
    removeReactionData: RoomRemoveMessageReactionPayload
  ): Promise<boolean> {
    const senderCommunicationID = removeReactionData.senderID;
    const matrixAgent = await this.acquireMatrixAgent(senderCommunicationID);

    await this.matrixUserAdapter.verifyRoomMembershipOrFail(
      matrixAgent.matrixClient,
      removeReactionData.roomID
    );
    this.logger.verbose?.(
      `[Adding reaction] Removing reaction to message in room: ${removeReactionData.roomID}`,
      LogContext.COMMUNICATION
    );
    try {
      await this.matrixAgentService.removeReactionOnMessage(
        matrixAgent,
        removeReactionData.roomID,
        removeReactionData.reactionID
      );
    } catch (error: any) {
      this.logger.error(
        `[Message sending] Unable to add reaction to message for user (${senderCommunicationID}): ${error}`,
        LogContext.COMMUNICATION
      );
      throw error;
    }
    this.logger.verbose?.(
      `...reaction added to message in room: ${removeReactionData.roomID}`,
      LogContext.COMMUNICATION
    );

    return true;
  }

  async editMessage(
    editMessageData: CommunicationEditMessageInput
  ): Promise<void> {
    const matrixAgent = await this.acquireMatrixAgent(
      editMessageData.senderCommunicationsID
    );

    await this.matrixAgentService.editMessage(
      matrixAgent,
      editMessageData.roomID,
      editMessageData.messageId,
      {
        text: editMessageData.message,
      }
    );
  }

  async deleteMessage(
    deleteMessageData: RoomDeleteMessagePayload
  ): Promise<string> {
    // when deleting a message use the global admin account
    // the possibility to use native matrix power levels is there
    // but we still don't have the infrastructure to support it
    const matrixAgentElevated = await this.getMatrixManagementAgentElevated();

    const matrixClient = matrixAgentElevated.matrixClient;
    const adminUser = this.getUserIdFromMatrixClient(matrixClient);

    const roomID = deleteMessageData.roomID;
    await this.addUserToRoom(roomID, adminUser);
    this.logger.verbose?.(`User added to ${roomID} room`);

    await this.matrixAgentService.deleteMessage(
      matrixAgentElevated,
      deleteMessageData.roomID,
      deleteMessageData.messageID
    );
    return deleteMessageData.messageID;
  }

  async sendMessageToUser(
    sendMessageUserData: SendMessageToUserPayload
  ): Promise<string> {
    const matrixAgent = await this.acquireMatrixAgent(
      sendMessageUserData.senderID
    );

    // todo: not always reinitiate the room connection
    const roomID = await this.matrixAgentService.initiateMessagingToUser(
      matrixAgent,
      {
        text: '',
        matrixID: sendMessageUserData.receiverID,
      }
    );

    const messageId = await this.matrixAgentService.sendMessage(
      matrixAgent,
      roomID,
      {
        text: sendMessageUserData.message,
      }
    );

    return messageId;
  }

  async getMessageSender(roomID: string, messageID: string): Promise<string> {
    // only the admin agent has knowledge of all rooms and synchronizes the state
    const matrixElevatedAgent = await this.getMatrixManagementAgentElevated();
    const matrixRoom = await this.matrixAgentService.getRoom(
      matrixElevatedAgent,
      roomID
    );

    const messages =
      await this.matrixRoomAdapter.getMatrixRoomTimelineAsMessages(
        matrixElevatedAgent.matrixClient,
        matrixRoom
      );
    const matchingMessage = messages.find(message => message.id === messageID);
    if (!matchingMessage) {
      throw new MatrixEntityNotFoundException(
        `Unable to locate message (id: ${messageID}) in room: ${matrixRoom.name} (${roomID})`,
        LogContext.COMMUNICATION
      );
    }

    return matchingMessage.sender;
  }

  async getReactionSender(roomID: string, reactionID: string): Promise<string> {
    // only the admin agent has knowledge of all rooms and synchronizes the state
    const matrixElevatedAgent = await this.getMatrixManagementAgentElevated();
    const matrixRoom = await this.matrixAgentService.getRoom(
      matrixElevatedAgent,
      roomID
    );

    const reactions =
      await this.matrixRoomAdapter.getMatrixRoomTimelineReactions(
        matrixElevatedAgent.matrixClient,
        matrixRoom
      );
    const matchingReaction = reactions.find(
      reaction => reaction.id === reactionID
    );
    if (!matchingReaction) {
      throw new MatrixEntityNotFoundException(
        `Unable to locate message (id: ${reactionID}) in room: ${matrixRoom.name} (${roomID})`,
        LogContext.COMMUNICATION
      );
    }

    return matchingReaction.sender;
  }

  private async acquireMatrixAgent(matrixUserId: string) {
    if (!this.enabled) {
      throw new NotEnabledException(
        'Communications not enabled',
        LogContext.COMMUNICATION
      );
    }
    return await this.matrixAgentPool.acquire(matrixUserId);
  }

  private async getGlobalAdminUser() {
    if (this.adminUser) {
      return this.adminUser;
    }

    const adminExists = await this.matrixUserManagementService.isRegistered(
      this.adminCommunicationsID
    );
    if (adminExists) {
      this.logger.verbose?.(
        `Admin user is registered: ${this.adminEmail}, logging in...`,
        LogContext.COMMUNICATION
      );
      const adminUser = await this.matrixUserManagementService.login(
        this.adminCommunicationsID,
        this.adminPassword
      );
      this.adminUser = adminUser;
      return adminUser;
    }

    this.adminUser = await this.registerNewAdminUser();
    return this.adminUser;
  }

  private async getMatrixManagementAgentElevated() {
    if (this.matrixElevatedAgent) {
      return this.matrixElevatedAgent;
    }

    const adminUser = await this.getGlobalAdminUser();
    this.matrixElevatedAgent = await this.matrixAgentService.createMatrixAgent(
      adminUser
    );

    await this.matrixElevatedAgent.start({
      registerTimelineMonitor: false,
      registerRoomMonitor: false,
    });

    return this.matrixElevatedAgent;
  }

  private async logServerVersion() {
    try {
      const version = await this.matrixUserManagementService.getServerVersion();
      this.logger.verbose?.(
        `Synapse server version: ${version}`,
        LogContext.BOOTSTRAP
      );
    } catch (error: any) {
      this.logger.verbose?.(
        `Unable to get synapse server version: ${error}`,
        LogContext.BOOTSTRAP
      );
    }
  }

  private async registerNewAdminUser(): Promise<IOperationalMatrixUser> {
    return await this.matrixUserManagementService.register(
      this.adminCommunicationsID,
      this.adminPassword,
      true
    );
  }

  async tryRegisterNewUser(email: string): Promise<string | undefined> {
    try {
      const matrixUserID = this.matrixUserAdapter.convertEmailToMatrixID(email);

      const isRegistered = await this.matrixUserManagementService.isRegistered(
        matrixUserID
      );

      if (!isRegistered) {
        await this.matrixUserManagementService.register(matrixUserID);
      }

      return matrixUserID;
    } catch (error) {
      this.logger.verbose?.(
        `Attempt to register user failed: ${error}; user registration for Communication to be re-tried later`,
        LogContext.COMMUNICATION
      );
    }
  }

  async createCommunityRoom(
    name: string,
    metadata?: Record<string, string>
  ): Promise<string> {
    // If not enabled just return an empty string
    if (!this.enabled) {
      return '';
    }
    const elevatedMatrixAgent = await this.getMatrixManagementAgentElevated();
    const room = await this.matrixRoomAdapter.createRoom(
      elevatedMatrixAgent.matrixClient,
      {
        metadata,
        createOpts: {
          name,
        },
      }
    );
    this.logger.verbose?.(
      `Created community room: ${room}`,
      LogContext.COMMUNICATION
    );
    return room;
  }

  async grantUserAccesToRooms(
    roomIDs: string[],
    matrixUserID: string
  ): Promise<boolean> {
    // If not enabled just return
    if (!this.enabled) {
      return false;
    }
    try {
      await this.addUserToRooms(roomIDs, matrixUserID);
    } catch (error) {
      this.logger.warn?.(
        `Unable to add user (${matrixUserID}) to rooms (${roomIDs}): already added?: ${error}`,
        LogContext.COMMUNICATION
      );
      return false;
    }
    return true;
  }

  async getCommunityRooms(matrixUserID: string): Promise<RoomResult[]> {
    const rooms: RoomResult[] = [];
    // If not enabled just return an empty array
    if (!this.enabled) {
      return rooms;
    }

    this.logger.verbose?.(
      `Retrieving rooms for user: ${matrixUserID}`,
      LogContext.COMMUNICATION
    );
    // use the global admin account when reading the contents of rooms
    // as protected via the graphql api
    // the possibility to use native matrix power levels is there
    // but we still don't have the infrastructure to support it
    // const matrixAgentElevated = await this.getMatrixManagementAgentElevated();

    // const matrixAgent = await this.matrixAgentPool.acquire(matrixUserID);

    // const matrixCommunityRooms =
    //   await this.matrixAgentService.getCommunityRooms(matrixAgent);
    // for (const matrixRoom of matrixCommunityRooms) {
    //   const room =
    //     await this.matrixRoomAdapter.convertMatrixRoomToCommunityRoom(
    //       matrixAgentElevated.matrixClient,
    //       matrixRoom
    //     );
    //   rooms.push(room);
    // }
    return rooms;
  }

  async getDirectRooms(matrixUserID: string): Promise<RoomDirectResult[]> {
    const rooms: RoomDirectResult[] = [];
    // If not enabled just return an empty array
    if (!this.enabled) {
      return rooms;
    }
    const matrixAgent = await this.matrixAgentPool.acquire(matrixUserID);

    const matrixDirectRooms = await this.matrixAgentService.getDirectRooms(
      matrixAgent
    );
    for (const matrixRoom of matrixDirectRooms) {
      // todo: likely a bug in the email mapping below
      const room = await this.matrixRoomAdapter.convertMatrixRoomToDirectRoom(
        matrixAgent.matrixClient,
        matrixRoom,
        matrixRoom.receiverCommunicationsID || ''
      );
      rooms.push(room);
    }

    return rooms;
  }

  async removeUserFromRooms(
    roomIDs: string[],
    matrixUserID: string
  ): Promise<boolean> {
    // If not enabled just return
    if (!this.enabled) {
      return false;
    }
    this.logger.verbose?.(
      `Removing user (${matrixUserID}) from rooms (${roomIDs})`,
      LogContext.COMMUNICATION
    );
    const userAgent = await this.matrixAgentPool.acquire(matrixUserID);
    const matrixAgentElevated = await this.getMatrixManagementAgentElevated();
    for (const roomID of roomIDs) {
      // added this for logging purposes
      userAgent.attachOnceConditional({
        id: roomID,
        roomMemberMembershipMonitor:
          userAgent.resolveForgetRoomMembershipOneTimeMonitor(
            roomID,
            matrixUserID,
            // once we have forgotten the room detach the subscription
            () => userAgent.detach(roomID),
            () => this.logger.verbose?.('completed'),
            () => this.logger.verbose?.('rejected')
          ) as any,
      });
      await this.matrixRoomAdapter.removeUserFromRoom(
        matrixAgentElevated.matrixClient,
        roomID,
        userAgent.matrixClient
      );
    }
    return true;
  }

  async getAllRooms(): Promise<RoomResult[]> {
    const elevatedAgent = await this.getMatrixManagementAgentElevated();
    this.logger.verbose?.(
      `[Admin] Obtaining all rooms on Matrix instance using ${elevatedAgent.matrixClient.getUserId()}`,
      LogContext.COMMUNICATION
    );
    const rooms = await elevatedAgent.matrixClient.getRooms();
    const roomResults: RoomResult[] = [];
    for (const room of rooms) {
      // Only count rooms with at least one member that is not the elevated agent
      const memberIDs = await this.matrixRoomAdapter.getMatrixRoomMembers(
        elevatedAgent.matrixClient,
        room.roomId
      );
      if (memberIDs.length === 0) continue;
      if (memberIDs.length === 1) {
        if (memberIDs[0] === elevatedAgent.matrixClient.getUserId()) continue;
      }
      const roomResult = new RoomResult();
      roomResult.id = room.roomId;
      roomResult.displayName = room.name;
      roomResult.members = memberIDs;
      roomResults.push(roomResult);
    }

    return roomResults;
  }

  async replicateRoomMembership(
    targetRoomID: string,
    sourceRoomID: string,
    userToPrioritize: string
  ): Promise<boolean> {
    try {
      this.logger.verbose?.(
        `[Replication] Replicating room membership from ${sourceRoomID} to ${targetRoomID}`,
        LogContext.COMMUNICATION
      );
      const elevatedAgent = await this.getMatrixManagementAgentElevated();

      const sourceMatrixUserIDs =
        await this.matrixRoomAdapter.getMatrixRoomMembers(
          elevatedAgent.matrixClient,
          sourceRoomID
        );

      // Ensure the user to be prioritized is first
      const userIndex = sourceMatrixUserIDs.findIndex(
        userID => userID === userToPrioritize
      );
      if (userIndex !== -1) {
        sourceMatrixUserIDs.splice(0, 0, userToPrioritize);
        sourceMatrixUserIDs.splice(userIndex + 1, 1);
      }

      for (const matrixUserID of sourceMatrixUserIDs) {
        // skip the matrix elevated agent
        if (matrixUserID === elevatedAgent.matrixClient.getUserId()) continue;
        const userAgent = await this.acquireMatrixAgent(matrixUserID);

        const userID = userAgent.matrixClient.getUserId();
        if (!userID) {
          throw new MatrixEntityNotFoundException(
            `Unable to retrieve user on agent: ${userAgent}`,
            LogContext.MATRIX
          );
        }
        userAgent.attachOnceConditional({
          id: targetRoomID,
          roomMemberMembershipMonitor:
            userAgent.resolveAutoAcceptRoomMembershipOneTimeMonitor(
              // subscribe for events for a specific room
              targetRoomID,
              userID,
              // once we have joined the room detach the subscription
              () => userAgent.detach(targetRoomID)
            ),
        });

        await this.matrixRoomAdapter.inviteUserToRoom(
          elevatedAgent.matrixClient,
          targetRoomID,
          userAgent.matrixClient
        );
      }
    } catch (error) {
      this.logger.error?.(
        `Unable to duplicate room membership from (${sourceRoomID}) to (${targetRoomID}): ${error}`,
        LogContext.COMMUNICATION
      );
      return false;
    }
    return true;
  }

  public async addUserToRoom(roomID: string, matrixUserID: string) {
    let userAgent = await this.getMatrixManagementAgentElevated();
    if (matrixUserID !== this.adminCommunicationsID) {
      userAgent = await this.matrixAgentPool.acquire(matrixUserID);
    }

    let isUserMember = await this.matrixUserAdapter.isUserMemberOfRoom(
      userAgent.matrixClient,
      roomID
    );
    try {
      if (isUserMember === false) {
        this.logger.verbose?.(
          `User (${matrixUserID}) joining room: ${roomID} - invitation not yet accepted, waiting`,
          LogContext.COMMUNICATION
        );
        await this.matrixRoomAdapter.joinRoomSafe(userAgent, roomID);
        // TODO: temporary work around to allow room membership to complete
        const maxRetries = 30;
        let reTries = 0;
        while (!isUserMember && reTries < maxRetries) {
          await sleep(100);
          isUserMember = await this.matrixUserAdapter.isUserMemberOfRoom(
            userAgent.matrixClient,
            roomID
          );
          reTries++;
        }
      }
    } catch (ex: any) {
      this.logger.error?.(
        `[Membership] Exception user joining a room (user: ${matrixUserID}) room: ${roomID}) - ${ex.toString()}`,
        LogContext.COMMUNICATION
      );
    }
  }

  private async addUserToRooms(roomIDs: string[], matrixUserID: string) {
    const elevatedAgent = await this.getMatrixManagementAgentElevated();
    let userAgent = elevatedAgent;
    if (matrixUserID !== this.adminCommunicationsID) {
      userAgent = await this.matrixAgentPool.acquire(matrixUserID);
    }

    const roomsToAdd = await this.getRoomsUserIsNotMember(
      userAgent.matrixClient,
      roomIDs
    );

    if (roomsToAdd.length === 0) {
      // Nothing to do; avoid wait below...
      return;
    }

    for (const roomID of roomsToAdd) {
      this.logger.verbose?.(
        `[Membership] Inviting user (${matrixUserID}) is join room: ${roomID}`,
        LogContext.COMMUNICATION
      );
      const userID = this.getUserIdFromMatrixClient(userAgent.matrixClient);

      userAgent.attachOnceConditional({
        id: roomID,
        roomMemberMembershipMonitor:
          userAgent.resolveAutoAcceptRoomMembershipOneTimeMonitor(
            // subscribe for events for a specific room
            roomID,
            userID,
            // once we have joined the room detach the subscription
            () => userAgent.detach(roomID)
          ),
      });

      await this.matrixRoomAdapter.inviteUserToRoom(
        elevatedAgent.matrixClient,
        roomID,
        userAgent.matrixClient
      );
    }
  }

  public getUserIdFromMatrixClient(matrixClient: MatrixClient): string {
    const userID = matrixClient.getUserId();
    if (!userID) {
      throw new MatrixEntityNotFoundException(
        `Unable to retrieve user on agent: ${matrixClient}`,
        LogContext.MATRIX
      );
    }
    return userID;
  }

  async getRoomsUserIsNotMember(
    matrixClient: MatrixClient,
    roomIDs: string[]
  ): Promise<string[]> {
    // Filter down to exclude the rooms the user is already a member of
    const joinedRooms = await this.matrixUserAdapter.getJoinedRooms(
      matrixClient
    );
    const applicableRoomIDs = roomIDs.filter(
      rId => !joinedRooms.find(joinedRoomId => joinedRoomId === rId)
    );
    if (applicableRoomIDs.length == 0) {
      this.logger.verbose?.(
        `User (${matrixClient.getUserId()}) is already in all rooms: ${roomIDs}`,
        LogContext.COMMUNICATION
      );
      return [];
    }
    return applicableRoomIDs;
  }

  async removeRoom(matrixRoomID: string) {
    try {
      const elevatedAgent = await this.getMatrixManagementAgentElevated();
      this.logger.verbose?.(
        `[Membership] Removing members from matrix room: ${matrixRoomID}`,
        LogContext.COMMUNICATION
      );
      const room = await this.matrixRoomAdapter.getMatrixRoom(
        elevatedAgent.matrixClient,
        matrixRoomID
      );
      const members = room.getMembers();
      for (const member of members) {
        // ignore matrix admin
        if (member.userId === elevatedAgent.matrixClient.getUserId()) continue;
        const userAgent = await this.matrixAgentPool.acquire(member.userId);
        await this.matrixRoomAdapter.removeUserFromRoom(
          elevatedAgent.matrixClient,
          matrixRoomID,
          userAgent.matrixClient
        );
      }
      this.logger.verbose?.(
        `[Membership] Removed members from room: ${room.name}`,
        LogContext.COMMUNICATION
      );
    } catch (error) {
      this.logger.verbose?.(
        `Unable to remove room  (${matrixRoomID}): ${error}`,
        LogContext.COMMUNICATION
      );
      return false;
    }
    return true;
  }

  async getRoomMembers(matrixRoomID: string): Promise<string[]> {
    let userIDs: string[] = [];
    try {
      const elevatedAgent = await this.getMatrixManagementAgentElevated();
      this.logger.verbose?.(
        `Getting members of matrix room: ${matrixRoomID}`,
        LogContext.COMMUNICATION
      );
      userIDs = await this.matrixRoomAdapter.getMatrixRoomMembers(
        elevatedAgent.matrixClient,
        matrixRoomID
      );
    } catch (error) {
      this.logger.verbose?.(
        `Unable to get room members (${matrixRoomID}): ${error}`,
        LogContext.COMMUNICATION
      );
      throw error;
    }
    return userIDs;
  }

  async getRoomStateJoinRule(
    roomID: string,
    matrixAgent: MatrixAgent
  ): Promise<string> {
    return await this.matrixRoomAdapter.getJoinRule(
      matrixAgent.matrixClient,
      roomID
    );
  }

  async getRoomStateHistoryVisibility(
    roomID: string,
    matrixAgent: MatrixAgent
  ): Promise<HistoryVisibility> {
    return await this.matrixRoomAdapter.getHistoryVisibility(
      matrixAgent.matrixClient,
      roomID
    );
  }

  async updateRoomState(
    roomData: UpdateRoomStatePayload
  ): Promise<RoomDetailsResponsePayload> {
    const elevatedAgent = await this.getMatrixManagementAgentElevated();

    if (roomData.roomID.length === 0) {
      throw new MatrixEntityNotFoundException(
        `No room ID provided: ${JSON.stringify(roomData)}`,
        LogContext.COMMUNICATION
      );
    }

    try {
      const roomID = roomData.roomID;
      if (roomData.historyWorldVisibile) {
        await this.matrixRoomAdapter.changeRoomStateHistoryVisibility(
          elevatedAgent.matrixClient,
          roomID,
          // not sure where to find the enums - reverse engineered this from synapse
          roomData.historyWorldVisibile
            ? HistoryVisibility.WorldReadable
            : HistoryVisibility.Joined
        );

        const oldRoom = await this.matrixRoomAdapter.getMatrixRoom(
          elevatedAgent.matrixClient,
          roomID
        );
        const rule = oldRoom.getGuestAccess();
        this.logger.verbose?.(
          `Room ${roomID} guest access is now: ${rule}`,
          LogContext.COMMUNICATION
        );
      }

      if (roomData.allowJoining) {
        await this.matrixRoomAdapter.changeRoomStateJoinRule(
          elevatedAgent.matrixClient,
          roomID,
          // not sure where to find the enums - reverse engineered this from synapse
          roomData.allowJoining ? JoinRule.Public : JoinRule.Invite
        );

        const oldRoom = await this.matrixRoomAdapter.getMatrixRoom(
          elevatedAgent.matrixClient,
          roomID
        );
        const rule = oldRoom.getJoinRule();
        this.logger.verbose?.(
          `Room ${roomID} join rule is now: ${rule}`,
          LogContext.COMMUNICATION
        );
      }
    } catch (error) {
      this.logger.error?.(
        `Unable to change state access for rooms to (${roomData}}): ${error}`,
        LogContext.COMMUNICATION
      );
    }
    const roomDetails = await this.getCommunityRoom(roomData.roomID);
    const result: RoomDetailsResponsePayload = {
      room: roomDetails,
    };
    return result;
  }
}
