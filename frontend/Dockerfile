FROM node:22-alpine AS development

WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
EXPOSE 5173
CMD ["npm", "run", "dev"]

FROM development AS build
RUN npm run build

FROM nginx:1.27-alpine AS production
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
# Socket directory for the TLS edge (mounted from a volume in production). A
# socket left by an unclean stop would stop nginx from starting, so remove it
# before nginx binds.
RUN mkdir -p /run/edge \
    && printf '#!/bin/sh\nrm -f /run/edge/web.sock\n' > /docker-entrypoint.d/05-remove-stale-edge-socket.sh \
    && chmod +x /docker-entrypoint.d/05-remove-stale-edge-socket.sh
EXPOSE 80
