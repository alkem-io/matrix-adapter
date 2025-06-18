FROM node:20.19.1-alpine


# Create app directory
WORKDIR /usr/src/app

# Define graphql server port
ARG ENV_ARG=production

# Install app dependencies
# A wildcard is used to ensure both package.json AND package-lock.json are copied
# where available (npm@5+)
COPY ./service/package*.json ./

RUN npm i -g npm@10.5.0
RUN npm install

## Add the wait script to the image
ADD https://github.com/ufoscout/docker-compose-wait/releases/download/2.7.3/wait /wait
RUN chmod +x /wait

# If you are building your code for production
# RUN npm ci --only=production

# Bundle app source & config files for TypeORM & TypeScript
COPY ./service/src ./src
COPY ./service/tsconfig.json .
COPY ./service/tsconfig.build.json .
COPY ./service/matrix-adapter.yml .

RUN npm run build

ENV NODE_ENV=${ENV_ARG}

EXPOSE 4006

CMD ["/bin/sh", "-c", "npm run start:prod NODE_OPTIONS=--max-old-space-size=2048"]
