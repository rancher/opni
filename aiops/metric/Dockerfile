FROM python:3.8-slim

COPY ./ /app/
WORKDIR /app
RUN apt update
RUN apt install vim -y
RUN pip install --no-cache-dir -r requirements.txt

RUN chmod a+rwx -R /app

CMD ["sleep", "50000000000"]