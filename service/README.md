# Matrix Adapter

Alkemio Matrix Adapter service.

[![Build Status](https://app.travis-ci.com/alkem-io/matrix-adapter.svg?branch=develop)](https://app.travis-ci.com/alkem-io/matrix-adapter.svg?branch=develop)
[![Coverage Status](https://coveralls.io/repos/github/alkem-io/matrix-adapter/badge.svg?branch=develop)](https://coveralls.io/github/alkem-io/matrix-adapter?branch=develop)
[![BCH compliance](https://bettercodehub.com/edge/badge/alkem-io/matrix-adapter?branch=develop)](https://bettercodehub.com/)
[![Deploy to DockerHub](https://github.com/alkem-io/matrix-adapter/actions/workflows/build-release-docker-hub.yml/badge.svg)](https://github.com/alkem-io/matrix-adapter/actions/workflows/build-release-docker-hub.yml)

## To test2

1. Start quickstart-services from the server repo with defaults.
2. Go to http://localhost:15672/#/queues/%2F/alkemio-matrix-adapter.
3. Under publish message, go to `properties` and add a new property with name `content_type` and value `application/json`.
4. Select payload:

```json
{
  "pattern": { "cmd": "roomDetails" },
  "data": {
    "triggeredBy": "",
    "roomID": "!xfyCOHLZQkCkAeilNd:alkemio.matrix.host"
  },
  "id": "35285bf3-09d8-4cdf-983e-8ca5054491c2"
}
```

5. Click publish.

## To build a new docker image locally

Execute the following command from the workspace root:

`docker build -t alkemio/matrix-adapter:v0.2.0 .`

## Explainer

This service uses the Matrix JS SDK to create a client for user.

This is done in two ways:

- there is an elevated admin user that is used to create rooms
- each Alkemio user for sending messages to a room

The Matrix Client created for a user knows about all rooms **that the user is a member of**. This is key: if the user is not a member of a room then the client does not know about that room.

For this reason the resulting setup is that the admin user ends up being a member of many rooms, as the user is added to rooms to allow reading from the room. This means that the admin user can potentially take a long time to sync up.

To address this, an admin account is only added as a member at the moment that it needs to read the room. This does mean the sync time will increase over time.
However it is then trivial to go to a new admin account and have that gradually join the rooms that are needed.

### Usage of power levels

Each room has a state. This state specifies what is the power level needed for particular actions. And crucially what is the power level that is assigned to new members of the room.

The way Alkemio uses rooms is that new members of a room get a power level that is high enough so that they can delete / carry out other actions on the room.

In rooms created before mid-2024, the users_default power level was set to zero. This meant that they cannot delete messages. After this point the default power level for new users in a room is set to 100 so that they can carry out all actions.

The power levels in rooms will need to be tidied up before the rooms in Matrix could be exposed outside of the cluster.

References:

- https://spec.matrix.org/v1.3/client-server-api/#room-events and then search for "m.room.power_levels"
