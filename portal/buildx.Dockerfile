# syntax=docker/dockerfile:1.2
# Portal UI dependencies
FROM node:18-alpine AS portal-ui-deps

WORKDIR /app

COPY package.json yarn.lock ./

RUN yarn install

# Portal UI build
FROM node:18-alpine AS portal-ui-builder

WORKDIR /app

COPY --from=portal-ui-deps /app/node_modules ./node_modules
COPY . .

RUN yarn build

FROM scratch AS portal-ui-export

COPY --from=portal-ui-builder /app/build dist
COPY --from=portal-ui-deps /app/yarn.lock .