ARG ALPINE_VERSION=3.19
ARG GOLANG_VERSION=1.21
ARG NODE_VERSION=21

FROM node:$NODE_VERSION-alpine$ALPINE_VERSION AS uibuilder
ARG NODE_ENV=production
ENV NODE_ENV=$NODE_ENV
WORKDIR /ui
COPY ui/package.json ui/package-lock.json ./
RUN npm install --include=dev
COPY ui .
RUN npm run build

# Stage 1: Build the Go binary
FROM golang:$GOLANG_VERSION-alpine$ALPINE_VERSION AS builder

# Set the working directory
WORKDIR /app

# Copy the source code to the container
COPY . .

# Build the Go binary
RUN go build -o rds-scaler .

# Stage 2: Create the runtime image
FROM alpine:$ALPINE_VERSION as runner

# Set the working directory
WORKDIR /app

# Copy the binary from the build stage to the runtime stage
COPY --from=builder /app/rds-scaler .
COPY --from=uibuilder /ui/dist ./ui/build

# Install ca-certificates for SSL support (required for AWS SDK)
RUN apk add --no-cache ca-certificates

USER nobody
EXPOSE 8041

# Run the Go binary
CMD ["./rds-scaler"]
