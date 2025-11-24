# syntax=docker/dockerfile:1.9

FROM node:20-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm install
COPY . .
ARG VITE_USER_SERVICE_URL=http://localhost:8888
ARG VITE_GAME_SERVICE_URL=http://localhost:8890
ENV VITE_USER_SERVICE_URL=${VITE_USER_SERVICE_URL}
ENV VITE_GAME_SERVICE_URL=${VITE_GAME_SERVICE_URL}
RUN npm run build

FROM nginx:1.27-alpine
COPY --from=builder /app/dist /usr/share/nginx/html
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
