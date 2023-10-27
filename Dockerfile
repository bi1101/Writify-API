# Use an official Golang runtime as the parent image
FROM golang:1.20

# Set the working directory inside the container
WORKDIR /go/src/app

# Copy the local package files to the container's workspace.
COPY . .

# Install any needed dependencies specified in the Go modules.
RUN go mod download

# Build the Go app for production.
RUN go build

# Make the port 8080 available outside of the Docker container.
EXPOSE 8080

# Define environment variables, if needed.
# Example: ENV PORT 8080

# Command to run the application.
CMD ["./writify_api"]
