FROM node:24-alpine AS build

WORKDIR /site

COPY package.json package-lock.json ./
RUN npm ci

COPY . .
RUN npm run build

FROM nginxinc/nginx-unprivileged:1.30.4-alpine-slim AS runtime

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build --chown=101:101 /site/.vitepress/dist/ /usr/share/nginx/html/

USER 101:101

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["wget", "-qO-", "http://127.0.0.1:8080/"]
