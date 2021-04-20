IMAGE_NAME=amartyarancher/payload-receiver-service:v0.0
docker build .. -t $IMAGE_NAME -f ./Dockerfile

docker push $IMAGE_NAME
