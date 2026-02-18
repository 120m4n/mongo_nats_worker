#!/bin/bash

# Build and Push Script for TimescaleDB Writer Service
# Supports test (build) and production stages with linux/amd64 architecture

set -e

# Configuration
SERVICE_DIR="services/timescale-writer"
IMAGE_NAME="timescale-writer"
DOCKER_REGISTRY="docker.io"
DOCKER_USERNAME="${DOCKER_USERNAME:-electrosoftware}"
DOCKER_TAG="${DOCKER_TAG:-v1.0}"
PLATFORM="linux/amd64"
STAGE="${1:-test}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Functions
print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_docker() {
    if ! command -v docker &> /dev/null; then
        print_error "Docker is not installed or not in PATH"
        exit 1
    fi
    print_info "Docker found: $(docker --version)"
}

setup_buildx() {
    print_info "Setting up Docker Buildx..."
    
    # Create builder if it doesn't exist
    if ! docker buildx inspect multiarch-builder &> /dev/null; then
        print_info "Creating multiarch-builder..."
        docker buildx create --name multiarch-builder --driver docker-container --use
    else
        print_info "Using existing multiarch-builder"
        docker buildx use multiarch-builder
    fi
    
    docker buildx inspect --bootstrap
}

build_test_stage() {
    print_info "========================================="
    print_info "STAGE: TEST/BUILD"
    print_info "========================================="
    
    TEST_IMAGE="${DOCKER_USERNAME}/${IMAGE_NAME}:test"
    print_info "Building image for testing: ${TEST_IMAGE}"
    print_info "Platform: ${PLATFORM}"
    
    cd "${SERVICE_DIR}"
    
    # Build without pushing, load to local docker
    docker buildx build \
        --platform "${PLATFORM}" \
        --tag "${TEST_IMAGE}" \
        --load \
        .
    
    print_info "Test build completed successfully!"
    
    # Run basic checks
    print_info "Verifying image..."
    docker images | grep "${IMAGE_NAME}"
    
    # Optional: Run basic container test
    print_info "Running basic container test..."
    docker run --rm "${TEST_IMAGE}" --version || true
    
    print_info "Test stage completed successfully!"
    cd - > /dev/null
}

build_production_stage() {
    print_info "========================================="
    print_info "STAGE: PRODUCTION"
    print_info "========================================="
    
    print_info "Using Docker Hub username: ${DOCKER_USERNAME}"
    
    # Login to Docker Hub
    print_info "Logging in to Docker Hub..."
    if [ -n "${DOCKER_PASSWORD}" ]; then
        echo "${DOCKER_PASSWORD}" | docker login "${DOCKER_REGISTRY}" -u "${DOCKER_USERNAME}" --password-stdin
    else
        print_warn "DOCKER_PASSWORD not set, attempting interactive login..."
        docker login "${DOCKER_REGISTRY}" -u "${DOCKER_USERNAME}"
    fi
    
    # Build full image name
    FULL_IMAGE_NAME="${DOCKER_REGISTRY}/${DOCKER_USERNAME}/${IMAGE_NAME}"
    
    print_info "Building and pushing image: ${FULL_IMAGE_NAME}:${DOCKER_TAG}"
    print_info "Platform: ${PLATFORM}"
    
    cd "${SERVICE_DIR}"
    
    # Build and push to registry
    docker buildx build \
        --platform "${PLATFORM}" \
        --tag "${FULL_IMAGE_NAME}:${DOCKER_TAG}" \
        --tag "${FULL_IMAGE_NAME}:latest" \
        --push \
        .
    
    print_info "Production build and push completed successfully!"
    print_info "Image available at: ${FULL_IMAGE_NAME}:${DOCKER_TAG}"
    
    cd - > /dev/null
}

show_usage() {
    cat << EOF
Usage: ./build_and_push_timescale.sh [STAGE]

STAGES:
  test        Build and test image locally (default)
  production  Build and push to Docker Hub

ENVIRONMENT VARIABLES:
  DOCKER_USERNAME   Docker Hub username (default: electrosoftware)
  DOCKER_PASSWORD   Docker Hub password (optional, will prompt if not set)
  DOCKER_TAG        Image tag (default: v1.0)

EXAMPLES:
  # Test build (creates electrosoftware/timescale-writer:test)
  ./build_and_push_timescale.sh test

  # Production build with defaults (creates electrosoftware/timescale-writer:v1.0)
  ./build_and_push_timescale.sh production

  # Production with custom tag
  DOCKER_TAG=v1.1 ./build_and_push_timescale.sh production

  # Production with custom username and tag
  DOCKER_USERNAME=myuser DOCKER_TAG=v2.0.0 ./build_and_push_timescale.sh production

  # Production with password (non-interactive)
  DOCKER_PASSWORD=mypass ./build_and_push_timescale.sh production

EOF
}

# Main execution
main() {
    print_info "TimescaleDB Writer - Build and Push Script"
    print_info "Stage: ${STAGE}"
    print_info "Target Platform: ${PLATFORM}"
    echo ""
    
    # Validate stage
    if [[ "${STAGE}" != "test" && "${STAGE}" != "production" ]]; then
        print_error "Invalid stage: ${STAGE}"
        echo ""
        show_usage
        exit 1
    fi
    
    # Check prerequisites
    check_docker
    setup_buildx
    
    # Execute appropriate stage
    case "${STAGE}" in
        test)
            build_test_stage
            print_info "✅ Test stage completed successfully!"
            ;;
        production)
            build_production_stage
            print_info "✅ Production stage completed successfully!"
            print_info "🚀 Image pushed to Docker Hub"
            ;;
    esac
}

# Run main function
main
