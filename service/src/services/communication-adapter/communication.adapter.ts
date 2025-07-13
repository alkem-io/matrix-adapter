import { LogContext } from '@common/enums/index';
import { MatrixEntityNotFoundException } from '@common/exceptions/matrix.entity.not.found.exception';
import { MatrixAgentPool } from '@src/domain/matrix/agent-pool/matrix.agent.pool';
import { HistoryVisibility, JoinRule, MatrixClient } from 'matrix-js-sdk';
import { WINSTON_MODULE_NEST_PROVIDER } from 'nest-winston';
import { MatrixRoomAdapter } from '@src/domain/matrix/adapter-room/matrix.room.adapter';
import { MatrixUserAdapter } from '@src/domain/matrix/adapter-user/matrix.user.adapter';
import { MatrixAgent } from '@src/domain/agent/matrix.agent';
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
import { sleep } from 'matrix-js-sdk/lib/utils.js';
import { MatrixAdminUserElevatedService } from '../../domain/matrix-admin/user-elevated/matrix.admin.user.elevated.service';
import { MatrixAdminUserService } from '../../domain/matrix-admin/user/matrix.admin.user.service';
import pkg  from '@nestjs/common';
const { Inject, Injectable } = pkg;

@Injectable()
export class CommunicationAdapter {
  constructor(
    @Inject(WINSTON_MODULE_NEST_PROVIDER)
    private readonly logger: pkg.LoggerService,
    private matrixAgentPool: MatrixAgentPool,
    private matrixUserAdapter: MatrixUserAdapter,
    private matrixRoomAdapter: MatrixRoomAdapter,
    private userManagementService: MatrixAdminUserService,
    private userElevatedManagementService: MatrixAdminUserElevatedService
  ) {}

  async getCommunityRoom(
    roomID: string,
    withState = false
  ): Promise<RoomResult> {
    const matrixAgentElevated =
      await this.userElevatedManagementService.getMatrixAgentElevated();
    await this.ensureMatrixClientIsMemberOfRoom(
      matrixAgentElevated.matrixClient,
      roomID
    );

    const matrixRoom = await this.matrixRoomAdapter.getRoom(
      matrixAgentElevated,
      roomID
    );
    const result =
      await this.matrixRoomAdapter.convertMatrixRoomToCommunityRoom(
        matrixAgentElevated.matrixClient,
        matrixRoom
      );
    if (withState) {
      result.joinRule = await this.getRoomStateJoinRule(
        roomID,
        matrixAgentElevated
      );
      result.historyVisibility = await this.getRoomStateHistoryVisibility(
        roomID,
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
    const matrixAgent = await this.getUserAgent(senderCommunicationID);

    await this.ensureMatrixClientIsMemberOfRoom(
      matrixAgent.matrixClient,
      sendMessageData.roomID
    );
    this.logger.verbose?.(
      `[Message sending] Sending message to room: ${sendMessageData.roomID}`,
      LogContext.COMMUNICATION
    );
    let messageId = '';
    try {
      messageId = await this.matrixRoomAdapter.sendMessage(
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
    const matrixAgent = await this.getUserAgent(senderCommunicationID);

    await this.ensureMatrixClientIsMemberOfRoom(
      matrixAgent.matrixClient,
      sendMessageData.roomID
    );
    this.logger.verbose?.(
      `[Message sending] Sending message to room: ${sendMessageData.roomID}`,
      LogContext.COMMUNICATION
    );
    let messageId = '';
    try {
      messageId = await this.matrixRoomAdapter.sendReplyToMessage(
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
    const matrixAgent = await this.getUserAgent(senderCommunicationID);

    await this.ensureMatrixClientIsMemberOfRoom(
      matrixAgent.matrixClient,
      addReactionData.roomID
    );
    this.logger.verbose?.(
      `[Adding reaction] Adding reaction to message in room: ${addReactionData.roomID}`,
      LogContext.COMMUNICATION
    );
    let reactionId = '';
    try {
      reactionId = await this.matrixRoomAdapter.addReactionOnMessage(
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
    const matrixAgent = await this.getUserAgent(senderCommunicationID);

    await this.ensureMatrixClientIsMemberOfRoom(
      matrixAgent.matrixClient,
      removeReactionData.roomID
    );
    this.logger.verbose?.(
      `[Adding reaction] Removing reaction to message in room: ${removeReactionData.roomID}`,
      LogContext.COMMUNICATION
    );
    try {
      await this.matrixRoomAdapter.removeReactionOnMessage(
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
    const matrixAgent = await this.getUserAgent(
      editMessageData.senderCommunicationsID
    );

    await this.matrixRoomAdapter.editMessage(
      matrixAgent,
      editMessageData.roomID,
      editMessageData.messageId,
      {
        text: editMessageData.message,
      }
    );
  }
  public async ensureMatrixClientIsMemberOfRoom(
    matrixClient: MatrixClient,
    roomID: string
  ): Promise<void> {
    try {
      const user = this.getUserIdFromMatrixClient(matrixClient);
      await this.addUserToRoom(roomID, user);
      this.logger.verbose?.(
        `Admin (${user}) added to room: ${roomID}`,
        LogContext.COMMUNICATION
      );
    } catch (error) {
      const errorMessage = `Unable to add user ${this.getUserIdFromMatrixClient(
        matrixClient
      )} to room: ${error}`;
      this.logger.error(errorMessage, LogContext.COMMUNICATION);
      throw error;
    }
  }

  async deleteMessage(
    deleteMessageData: RoomDeleteMessagePayload
  ): Promise<string> {
    // when deleting a message use the global admin account
    // the possibility to use native matrix power levels is there
    // but we still don't have the infrastructure to support it
    // const matrixAgentElevated =
    //   await this.communicationAdminUserService.getMatrixManagementAgentElevated();

    // Prefer to delete with the current user to ensure audit trail is maintained in matrix
    const matrixAgent = await this.getUserAgent(deleteMessageData.triggeredBy);

    await this.ensureMatrixClientIsMemberOfRoom(
      matrixAgent.matrixClient,
      deleteMessageData.roomID
    );

    await this.matrixRoomAdapter.deleteMessage(
      matrixAgent,
      deleteMessageData.roomID,
      deleteMessageData.messageID
    );
    return deleteMessageData.messageID;
  }

  async sendMessageToUser(
    sendMessageUserData: SendMessageToUserPayload
  ): Promise<string> {
    const matrixAgent = await this.getUserAgent(sendMessageUserData.senderID);

    // todo: not always reinitiate the room connection
    const roomID = await this.matrixRoomAdapter.initiateMessagingToUser(
      matrixAgent,
      {
        text: '',
        matrixID: sendMessageUserData.receiverID,
      }
    );

    const messageId = await this.matrixRoomAdapter.sendMessage(
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
    const matrixElevatedAgent =
      await this.userElevatedManagementService.getMatrixAgentElevated();
    await this.ensureMatrixClientIsMemberOfRoom(
      matrixElevatedAgent.matrixClient,
      roomID
    );
    const matrixRoom = await this.matrixRoomAdapter.getRoom(
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
    const matrixElevatedAgent =
      await this.userElevatedManagementService.getMatrixAgentElevated();
    const matrixRoom = await this.matrixRoomAdapter.getRoom(
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

  async tryRegisterNewUser(email: string): Promise<string | undefined> {
    try {
      const matrixUserID = this.matrixUserAdapter.convertEmailToMatrixID(email);

      const isRegistered =
        await this.userManagementService.isRegistered(matrixUserID);

      if (!isRegistered) {
        await this.userManagementService.register(matrixUserID);
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
    const elevatedMatrixAgent =
      await this.userElevatedManagementService.getMatrixAgentElevated();
    const room = await this.matrixRoomAdapter.createRoom(
      elevatedMatrixAgent.matrixClient,
      {
        name,
      },
      {
        metadata,
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

    this.logger.verbose?.(
      `Retrieving rooms for user: ${matrixUserID}`,
      LogContext.COMMUNICATION
    );
    // use the global admin account when reading the contents of rooms
    // as protected via the graphql api
    // the possibility to use native matrix power levels is there
    // but we still don't have the infrastructure to support it
    // const matrixAgentElevated = await this.communicationAdminUserService.getMatrixManagementAgentElevated();

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

    const matrixAgent = await this.getUserAgent(matrixUserID);

    const matrixDirectRooms =
      await this.matrixRoomAdapter.getDirectRooms(matrixAgent);
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
    this.logger.verbose?.(
      `Removing user (${matrixUserID}) from rooms (${roomIDs})`,
      LogContext.COMMUNICATION
    );
    const userAgent = await this.getUserAgent(matrixUserID);
    const matrixAgentElevated =
      await this.userElevatedManagementService.getMatrixAgentElevated();
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
    const elevatedAgent =
      await this.userElevatedManagementService.getMatrixAgentElevated();

    this.logger.verbose?.(
      `[Admin] Obtaining all rooms on Matrix instance using ${elevatedAgent.matrixClient.getUserId()}`,
      LogContext.COMMUNICATION
    );

    const roomResults: RoomResult[] = [];

    // USING SLIDING SYNC PAGINATED ROOM ACCESS
    if ((elevatedAgent as any).slidingWindowManager) {
      const totalRooms = await (elevatedAgent as any).slidingWindowManager.getTotalRoomCount();
      const pageSize = 50;

      this.logger.verbose?.(
        `[Admin] Using sliding sync to get ${totalRooms} rooms in pages of ${pageSize}`,
        LogContext.COMMUNICATION
      );

      for (let offset = 0; offset < totalRooms; offset += pageSize) {
        const rooms = await (elevatedAgent as any).slidingWindowManager.getRoomList(offset, pageSize);

        for (const room of rooms) {
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
      }
    } else {
      this.logger.warn?.(
        '[Admin] Sliding window manager not available, cannot retrieve rooms',
        LogContext.COMMUNICATION
      );
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
      const elevatedAgent =
        await this.userElevatedManagementService.getMatrixAgentElevated();

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
        const userAgent = await this.getUserAgent(matrixUserID);

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

  // Gets the user agent, potentially returning the elevated agent if there is a match
  private async getUserAgent(matrixUserID: string): Promise<MatrixAgent> {
    if (
      matrixUserID === this.userElevatedManagementService.adminCommunicationsID
    ) {
      return await this.userElevatedManagementService.getMatrixAgentElevated();
    }
    return await this.matrixAgentPool.acquire(matrixUserID);
  }

  public async addUserToRoom(roomID: string, matrixUserID: string) {
    const userAgent = await this.getUserAgent(matrixUserID);
    let isUserMember = await this.matrixRoomAdapter.isUserMemberOfRoom(
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
          isUserMember = await this.matrixRoomAdapter.isUserMemberOfRoom(
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
    const elevatedAgent =
      await this.userElevatedManagementService.getMatrixAgentElevated();
    let userAgent = elevatedAgent;
    if (
      matrixUserID !== this.userElevatedManagementService.adminCommunicationsID
    ) {
      userAgent = await this.getUserAgent(matrixUserID);
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
    const joinedRooms =
      await this.matrixRoomAdapter.getJoinedRooms(matrixClient);
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
      const elevatedAgent =
        await this.userElevatedManagementService.getMatrixAgentElevated();
      this.logger.verbose?.(
        `[Membership] Removing matrix room: ${matrixRoomID}`,
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
        const userAgent = await this.getUserAgent(member.userId);
        await this.matrixRoomAdapter.removeUserFromRoom(
          elevatedAgent.matrixClient,
          matrixRoomID,
          userAgent.matrixClient
        );
      }
      this.logger.verbose?.(
        `[Membership] Removed matrix room: ${room.name}`,
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
      const elevatedAgent =
        await this.userElevatedManagementService.getMatrixAgentElevated();
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
    const elevatedAgent =
      await this.userElevatedManagementService.getMatrixAgentElevated();

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
