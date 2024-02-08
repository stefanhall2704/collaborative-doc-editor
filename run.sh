#!/bin/bash

# Navigate to the project root directory
cd "$(dirname "$0")"

# Build the project
echo "Building the project..."
go build -o collaborativeDocEditor ./cmd/server

# Run the executable
echo "Running the application..."
./collaborativeDocEditor
