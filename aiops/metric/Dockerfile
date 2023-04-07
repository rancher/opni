FROM rancher/opni-python-base:3.8-torch

COPY ./ /app/
WORKDIR /app
RUN pip install --no-cache-dir -r requirements.txt

RUN chmod a+rwx -R /app
EXPOSE 8090

CMD ["uvicorn", "backend:app","--host", "0.0.0.0", "--port", "8090"]