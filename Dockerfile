FROM node:20-alpine3.18 AS uibuilder
ARG NODE_ENV=production
ENV NODE_ENV=$NODE_ENV
WORKDIR /ui
COPY ui/package.json ui/package-lock.json ./
RUN npm install
COPY ui .
RUN npm run build

# Stage 1: Build the Go binary
FROM golang:1.21-alpine3.18 AS builder

# Set the working directory
WORKDIR /app

# Copy the source code to the container
COPY . .

# Build the Go binary
RUN go build -o rds-scaler .

# Stage 2: Create the runtime image
FROM alpine:3.18 as runner

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
