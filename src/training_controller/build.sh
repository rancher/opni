IMAGE_NAME=amartyarancher/training-controller:v0.1
docker build .. -t $IMAGE_NAME -f ./Dockerfile

docker push $IMAGE_NAME
