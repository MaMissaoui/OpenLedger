# syntax=docker/dockerfile:1

FROM node:22-alpine AS builder
RUN corepack enable && corepack prepare pnpm@latest --activate
WORKDIR /app
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY . .
# Authelia portal URL is baked into the bundle at build time.
ARG VITE_AUTHELIA_PORTAL_URL=http://auth.openledger.localhost
ENV VITE_AUTHELIA_PORTAL_URL=$VITE_AUTHELIA_PORTAL_URL
RUN pnpm build

FROM nginx:stable-alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
