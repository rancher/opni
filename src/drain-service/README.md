## DRAIN service

* This service controls the training and retraining of all other ML/DL models by sending a NATS signal after volatility log messages has decreased to an acceptable level
* Adopted from https://github.com/IBM/Drain3 and https://pinjiahe.github.io/papers/ICWS17.pdf

```
kubectl apply -f drain-service.yaml
```