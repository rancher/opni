IMAGE_NAME=sanjayrancher/preprocessing-service
docker build .. -t $IMAGE_NAME -f ./Dockerfile

docker push $IMAGE_NAME
