IMAGE_NAME=amartyarancher/nulog-train-amartya
docker build .. -t $IMAGE_NAME -f ./Dockerfile

docker push $IMAGE_NAME
