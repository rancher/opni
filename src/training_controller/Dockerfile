FROM nikolaik/python-nodejs:python3.8-nodejs14-slim
COPY requirements.txt ./
COPY training_controller.py ./
COPY prepare_training_logs.py ./
RUN pip install --no-cache-dir -r requirements.txt
RUN npm install elasticdump -g
CMD ["python", "training_controller.py"]