#!/bin/bash

# Set your Docker Hub username
DOCKER_USERNAME="jonathanleahy"
APP_NAME_FRONTEND="chat-frontend"
APP_NAME_BACKEND="chat-backend"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Function to print status messages
print_status() {
    echo -e "${GREEN}>>> $1${NC}"
}

# Function to print error messages
print_error() {
    echo -e "${RED}ERROR: $1${NC}"
    exit 1
}

# Function to clean up Kubernetes resources
cleanup_kubernetes() {
    print_status "Cleaning up Kubernetes resources..."

    # Delete deployments
    kubectl delete deployment chat-frontend --ignore-not-found
    kubectl delete deployment chat-backend --ignore-not-found
    kubectl delete deployment rabbitmq --ignore-not-found

    # Delete services
    kubectl delete service chat-frontend-service --ignore-not-found
    kubectl delete service chat-backend-service --ignore-not-found
    kubectl delete service rabbitmq-service --ignore-not-found

    # Delete any stuck pods
    kubectl delete pods -l app=chat-frontend --force --ignore-not-found
    kubectl delete pods -l app=chat-backend --force --ignore-not-found
    kubectl delete pods -l app=rabbitmq --force --ignore-not-found

    print_status "Cleanup completed"
}

# Build and push frontend
build_frontend() {
    print_status "Building frontend..."
    cd chat-frontend
    docker build -t $APP_NAME_FRONTEND:latest . || print_error "Frontend Docker build failed"
    docker tag $APP_NAME_FRONTEND:latest $DOCKER_USERNAME/$APP_NAME_FRONTEND:latest
    docker push $DOCKER_USERNAME/$APP_NAME_FRONTEND:latest || print_error "Frontend Docker push failed"
    cd ..
}

# Build and push backend
build_backend() {
    print_status "Building backend..."
    cd chat-backend
    docker build -t $APP_NAME_BACKEND:latest . || print_error "Backend Docker build failed"
    docker tag $APP_NAME_BACKEND:latest $DOCKER_USERNAME/$APP_NAME_BACKEND:latest
    docker push $DOCKER_USERNAME/$APP_NAME_BACKEND:latest || print_error "Backend Docker push failed"
    cd ..
}

# Clean up existing deployments
cleanup_kubernetes

# Build and push images
build_frontend
build_backend

# Apply Kubernetes configurations
print_status "Applying Kubernetes configurations..."

kubectl apply -f k8s/rabbitmq.yaml || print_error "Failed to apply RabbitMQ configuration"
kubectl apply -f k8s/backend.yaml || print_error "Failed to apply backend configuration"
kubectl apply -f k8s/frontend.yaml || print_error "Failed to apply frontend configuration"

# Wait for deployments to be ready
print_status "Waiting for deployments to be ready..."
kubectl rollout status deployment/chat-frontend --timeout=300s
kubectl rollout status deployment/chat-backend --timeout=300s
kubectl rollout status deployment/rabbitmq --timeout=300s

# Get service URLs
print_status "Getting service URLs..."
if command -v minikube &> /dev/null; then
    FRONTEND_URL=$(minikube service chat-frontend-service --url)
    BACKEND_URL=$(minikube service chat-backend-service --url)
    print_status "Frontend available at: ${FRONTEND_URL}"
    print_status "Backend available at: ${BACKEND_URL}"
else
    FRONTEND_IP=$(kubectl get service chat-frontend-service -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
    BACKEND_IP=$(kubectl get service chat-backend-service -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
    print_status "Frontend available at: http://${FRONTEND_IP}"
    print_status "Backend available at: http://${BACKEND_IP}"
fi

# Display pod status
echo ""
print_status "Pod Status:"
kubectl get pods -l app=chat-frontend
kubectl get pods -l app=chat-backend
kubectl get pods -l app=rabbitmq