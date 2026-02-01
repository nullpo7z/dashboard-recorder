#!/bin/bash
set -e

# Configuration
IMAGE_NAME="dashboard-recorder"

# 1. Get DockerHub Username
if [ -z "$1" ]; then
  echo -n "Enter your DockerHub Username: "
  read DOCKER_USER
else
  DOCKER_USER=$1
fi

if [ -z "$DOCKER_USER" ]; then
  echo "Error: Username is required."
  exit 1
fi

FULL_TAG="$DOCKER_USER/$IMAGE_NAME:latest"

# 2. Confirm
echo "----------------------------------------"
echo "Target Image: $FULL_TAG"
echo "----------------------------------------"
echo "This script will:"
echo "1. Build the Docker image"
echo "2. Push it to DockerHub"
echo ""
echo "Make sure you have run 'docker login' first."
echo "----------------------------------------"
echo -n "Continue? [y/N]: "
read CONFIRM
if [[ "$CONFIRM" != "y" && "$CONFIRM" != "Y" ]]; then
  echo "Aborted."
  exit 0
fi

# 3. Build
echo ">> Building image..."
docker build -t "$FULL_TAG" .

# 4. Push
echo ">> Pushing to DockerHub..."
docker push "$FULL_TAG"

echo ">> Success! Image published at: $FULL_TAG"
