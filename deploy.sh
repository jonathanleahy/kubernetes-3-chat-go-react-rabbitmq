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

# Function to print warning messages
print_warning() {
    echo -e "${YELLOW}WARNING: $1${NC}"
}

# Function to print error messages
print_error() {
    echo -e "${RED}ERROR: $1${NC}"
    exit 1
}

# Function to check minikube setup
check_minikube_setup() {
    print_status "Checking Minikube setup..."

    # Check if netcat is installed
    if ! command -v nc &> /dev/null; then
        print_status "Installing netcat for port checking..."
        if command -v apt-get &> /dev/null; then
            sudo apt-get update && sudo apt-get install -y netcat
        elif command -v yum &> /dev/null; then
            sudo yum install -y nc
        elif command -v brew &> /dev/null; then
            brew install netcat
        else
            print_warning "Could not install netcat. Port verification may be limited."
        fi
    fi

    # Check if minikube is running
    if ! minikube status | grep -q "Running"; then
        print_error "Minikube is not running. Please start minikube with: minikube start --addons=ingress --addons=registry --addons=metrics-server"
    fi

    # Check required addons
    local missing_addons=()

    # Check each addon
    if ! minikube addons list | grep "ingress" | grep -q "enabled"; then
        missing_addons+=("ingress")
    fi

    if ! minikube addons list | grep "registry" | grep -q "enabled"; then
        missing_addons+=("registry")
    fi

    if ! minikube addons list | grep "metrics-server" | grep -q "enabled"; then
        missing_addons+=("metrics-server")
    fi

    # If any addons are missing, show error
    if [ ${#missing_addons[@]} -ne 0 ]; then
        print_error "Required addons not enabled: ${missing_addons[*]}\nPlease restart minikube with: minikube start --addons=ingress --addons=registry --addons=metrics-server"
    fi

    # Check if CoreDNS is running
    if ! kubectl get pods -n kube-system | grep -q "coredns"; then
        print_error "CoreDNS is not running. Please restart minikube with: minikube start --addons=ingress --addons=registry --addons=metrics-server"
    fi

    print_status "Minikube setup verified successfully"
}

# Function to clean up Kubernetes resources
cleanup_kubernetes() {
    print_status "Cleaning up Kubernetes resources..."

    # Delete existing resources
    kubectl delete deployment chat-frontend chat-backend rabbitmq --ignore-not-found
    kubectl delete service chat-frontend-service chat-backend-service rabbitmq-service --ignore-not-found

    # Force delete any stuck pods
    kubectl delete pods -l app=chat-frontend --force --ignore-not-found
    kubectl delete pods -l app=chat-backend --force --ignore-not-found
    kubectl delete pods -l app=rabbitmq --force --ignore-not-found

    print_status "Cleanup completed"
}

# Function to wait for pod readiness
wait_for_pod_ready() {
    local label=$1
    local namespace=${2:-default}
    local timeout=${3:-300}

    print_status "Waiting for pod with label $label to be ready..."
    kubectl wait --for=condition=ready pod -l app=$label --timeout=${timeout}s || print_error "Pod $label not ready"
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

# Function to verify service accessibility with retries
verify_service() {
    local service_name=$1
    local port=$2
    local max_attempts=10  # Maximum number of retry attempts
    local wait_seconds=5   # Seconds to wait between attempts
    local attempt=1

    print_status "Verifying $service_name on port $port..."

    while [ $attempt -le $max_attempts ]; do
        case $service_name in
            chat-frontend-service)
                # HTTP check for frontend
                response=$(curl -s -o /dev/null -w "%{http_code}" http://$(minikube ip):$port)
                if [ "$response" == "200" ]; then
                    print_status "$service_name is accessible"
                    return 0
                fi
                ;;
            chat-backend-service)
                # For WebSocket endpoint, check if port is open
                if nc -z $(minikube ip) $port 2>/dev/null; then
                    print_status "$service_name is listening on port $port"
                    return 0
                fi
                ;;
            *)
                # Default TCP port check
                if nc -z $(minikube ip) $port 2>/dev/null; then
                    print_status "$service_name is listening on port $port"
                    return 0
                fi
                ;;
        esac

        print_status "Attempt $attempt/$max_attempts: Service not ready yet, waiting $wait_seconds seconds..."
        sleep $wait_seconds
        ((attempt++))
    done

    print_warning "$service_name not accessible after $max_attempts attempts, but continuing deployment..."
    return 1
}

# Check minikube setup first
check_minikube_setup

# Clean up existing deployments
cleanup_kubernetes

# Build and push images
build_frontend
build_backend

# Apply Kubernetes configurations
print_status "Applying Kubernetes configurations..."

# Deploy RabbitMQ
print_status "Deploying RabbitMQ..."
kubectl apply -f k8s/rabbitmq.yaml || print_error "Failed to apply RabbitMQ configuration"
wait_for_pod_ready "rabbitmq"

# Deploy backend
print_status "Deploying backend..."
kubectl apply -f k8s/backend.yaml || print_error "Failed to apply backend configuration"
wait_for_pod_ready "chat-backend"

# Deploy frontend
print_status "Deploying frontend..."
kubectl apply -f k8s/frontend.yaml || print_error "Failed to apply frontend configuration"
wait_for_pod_ready "chat-frontend"

# Get service URLs and status
print_status "Getting service URLs..."

# Display pod and service status
print_status "Pod Status:"
kubectl get pods
echo ""
print_status "Service Status:"
kubectl get svc

print_status "Waiting for services to be fully ready..."
sleep 10  # Initial wait for services to start

# Verify service accessibility
print_status "Verifying service accessibility..."
verify_service "chat-frontend-service" "30080"
verify_service "chat-backend-service" "30090"
verify_service "rabbitmq-service" "31672"

# Get Minikube IP
MINIKUBE_IP=$(minikube ip)

# Print access URLs
echo ""
print_status "Access URLs:"
echo "Frontend: http://${MINIKUBE_IP}:30080"
echo "Backend: http://${MINIKUBE_IP}:30090"
echo "RabbitMQ Management: http://${MINIKUBE_IP}:31672"

print_status "Deployment completed successfully!"