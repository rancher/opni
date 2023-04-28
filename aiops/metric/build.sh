cd ../..
dagger-cue do aiopsload
docker tag rancher/metric-ai-service  tybalex/metric-ai-service  &&  docker push tybalex/metric-ai-service

