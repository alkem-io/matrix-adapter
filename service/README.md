# Matrix Adapter

Alkemio Matrix Adapter service.

[![Build Status](https://app.travis-ci.com/alkem-io/matrix-adapter.svg?branch=develop)](https://app.travis-ci.com/alkem-io/matrix-adapter.svg?branch=develop)
[![Coverage Status](https://coveralls.io/repos/github/alkem-io/matrix-adapter/badge.svg?branch=develop)](https://coveralls.io/github/alkem-io/matrix-adapter?branch=develop)
[![BCH compliance](https://bettercodehub.com/edge/badge/alkem-io/matrix-adapter?branch=develop)](https://bettercodehub.com/)
[![Deploy to DockerHub](https://github.com/alkem-io/matrix-adapter/actions/workflows/build-release-docker-hub.yml/badge.svg)](https://github.com/alkem-io/matrix-adapter/actions/workflows/build-release-docker-hub.yml)

## To test

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
