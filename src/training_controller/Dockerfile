FROM nikolaik/python-nodejs:latest
COPY requirements.txt ./
COPY training_controller.py ./
COPY prepare_training_logs.py ./
RUN pip install -r requirements.txt
RUN npm install elasticdump -g
CMD ["python", "training_controller.py"]