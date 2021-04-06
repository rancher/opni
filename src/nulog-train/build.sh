IMAGE_NAME=amartyarancher/nulog-train
docker build .. -t $IMAGE_NAME -f ./Dockerfile

docker push $IMAGE_NAME
