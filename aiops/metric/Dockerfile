FROM rancher/opni-python-base:3.8-torch

COPY ./ /app/
WORKDIR /app
RUN pip install --no-cache-dir -r requirements.txt

RUN chmod a+rwx -R /app

CMD ["uvicorn", "backend:app", "--port", "10010"]