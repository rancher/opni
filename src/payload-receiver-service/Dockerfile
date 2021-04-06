FROM python:3.8
EXPOSE 80
COPY ./payload-receiver-service/app /app
COPY ./payload-receiver-service/requirements.txt /app
COPY ./utils/nats_wrapper.py /app/nats_wrapper.py
WORKDIR "app"
RUN pip install --no-cache-dir -r /app/requirements.txt
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "80"]
