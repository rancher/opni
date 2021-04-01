IMAGE_NAME=tybalex/nulog-inference-service
docker build . -t $IMAGE_NAME

docker push $IMAGE_NAME

