IMAGE_NAME=amartyarancher/nulog-train:v0.1
docker build .. -t $IMAGE_NAME -f ./Dockerfile

docker push $IMAGE_NAME