FROM nikolaik/python-nodejs:python3.8-nodejs14-slim
COPY ./ /app/

RUN chmod a+rwx -R /app
WORKDIR /app

RUN pip install --no-cache-dir -r requirements.txt
CMD ["python", "setup-default-opni-dashboard.py"]
