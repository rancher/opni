#IMAGE_NAME=tybalex/nulog-inference-service-control-plane:v0.1
IMAGE_NAME=sanjayrancher/nulog-inference-service:v0.0

docker build .. -t $IMAGE_NAME -f ./Dockerfile

docker push $IMAGE_NAME

