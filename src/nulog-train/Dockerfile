FROM python:3.8
COPY requirements.txt ./
COPY train.py ./
COPY NuLogModel.py ./
COPY NuLogParser.py ./
COPY NuLogTokenizer.py ./
COPY output ./output
COPY input ./input
ENV PATH /usr/local/nvidia/bin:/usr/local/cuda/bin:${PATH}
ENV LD_LIBRARY_PATH /usr/local/nvidia/lib:/usr/local/nvidia/lib64
ENV NVIDIA_VISIBLE_DEVICES all
ENV NVIDIA_DRIVER_CAPABILITIES compute,utility
RUN pip install --no-cache-dir -r requirements.txt
CMD ["python", "train.py"]
