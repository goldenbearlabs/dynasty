# 1. Build the frontend to static files.
FROM node:24-alpine AS frontend
WORKDIR /frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# 2. Build the Go binary with the frontend embedded.
FROM golang:1.26-alpine AS backend
WORKDIR /backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=frontend /frontend/build ./internal/web/dist
RUN CGO_ENABLED=0 go build -o /crossover .

# 3. Ship only the binary.
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata
COPY --from=backend /crossover /crossover
EXPOSE 8090
ENTRYPOINT ["/crossover"]
