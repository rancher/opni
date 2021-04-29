FROM nikolaik/python-nodejs:python3.8-nodejs14-slim
COPY ./training_controller/ /app/
COPY ./utils/nats_wrapper.py /app/nats_wrapper.py

RUN chmod a+rwx -R /app
WORKDIR /app

RUN pip install --no-cache-dir -r requirements.txt
RUN npm install elasticdump -g
CMD ["python", "training_controller.py"]