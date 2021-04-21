IMAGE_NAME=amartyarancher/preprocessing-service:v0.0
docker build .. -t $IMAGE_NAME -f ./Dockerfile

docker push $IMAGE_NAME
