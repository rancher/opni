IMAGE_NAME=tybalex/nulog-inference-service
docker build .. -t $IMAGE_NAME -f ./Dockerfile

docker push $IMAGE_NAME

