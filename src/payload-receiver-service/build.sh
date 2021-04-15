IMAGE_NAME=tybalex/payload-receiver-service
docker build .. -t $IMAGE_NAME -f ./Dockerfile

docker push $IMAGE_NAME
