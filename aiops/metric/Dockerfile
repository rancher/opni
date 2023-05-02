FROM rancher/opni-python-base:3.8-torch

COPY ./requirements.txt /app/requirements.txt
RUN pip install --no-cache-dir -r /app/requirements.txt

COPY ./metric_analysis /app/metric_analysis

WORKDIR /app/metric_analysis

RUN chmod a+rwx -R /app/metric_analysis
EXPOSE 8090

CMD ["uvicorn", "main:app","--host", "0.0.0.0", "--port", "8090"]
