FROM node:22.16.0-alpine


# Create app directory
WORKDIR /usr/src/app

# Define graphql server port
ARG ENV_ARG=production

# Install app dependencies
# where available (npm@5+)
COPY ./service/package.json ./
COPY ./service/pnpm-lock.yaml ./

RUN npm install -g pnpm
RUN pnpm install

## Add the wait script to the image
ADD https://github.com/ufoscout/docker-compose-wait/releases/download/2.7.3/wait /wait
RUN chmod +x /wait

# If you are building your code for production
# RUN npm ci --only=production

# Bundle app source & config files for TypeORM & TypeScript
COPY ./service/src ./src
COPY ./service/tsconfig.json .
COPY ./service/tsconfig.build.json .
COPY ./service/tsconfig.prod.json .
COPY ./service/nest-cli.json .
COPY ./service/.swcrc .
COPY ./service/matrix-adapter.yml .

RUN pnpm run build

ENV NODE_ENV=${ENV_ARG}

EXPOSE 4006

CMD ["/bin/sh", "-c", "npm run start:prod NODE_OPTIONS=--max-old-space-size=2048"]
