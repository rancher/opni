IMAGE_NAME=tybalex/nulog-inference-service-control-plane:v0.01
#IMAGE_NAME=tybalex/nulog-inference-service:v0.01

docker build .. -t $IMAGE_NAME -f ./Dockerfile

docker push $IMAGE_NAME

