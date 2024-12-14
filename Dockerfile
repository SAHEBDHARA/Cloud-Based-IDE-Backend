# Use a smaller base image for Go
FROM golang:1.22.2-alpine

# Install bash and other dependencies (if needed)
RUN apk add --no-cache bash

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum files to the working directory
COPY go.mod go.sum ./ 

# Download Go modules
RUN go mod tidy

# Copy the rest of the application code
COPY . .

# Build the Go application
RUN go build -o main ./main.go

# Make the binary executable
RUN chmod +x main

# Expose the application port
EXPOSE 9000

# Command to run the application
CMD ["./main"]
