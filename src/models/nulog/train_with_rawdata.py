# Standard Library
import json
import logging

# Third Party
from masker import LogMasker
from NuLogParser import LogParser

logging.basicConfig(level=logging.INFO, format="%(asctime)s - %(name)s - %(message)s")


def load_text():
    texts = []
    masker = LogMasker()
    with open("input/mix-raw.log", "r") as fin:
        for idx, line in enumerate(fin):
            try:
                log = json.loads(line)
                log = log["log"].rstrip().lower()
                log = masker.mask(log)
                texts.append(log)
            except Exception as e:
                logging.error(e)

    # texts = texts * 20
    return texts


def train_nulog_model():
    nr_epochs = 3
    num_samples = 0
    parser = LogParser()
    texts = load_text()

    tokenized = parser.tokenize_data(texts, isTrain=True)
    parser.tokenizer.save_vocab()
    parser.train(tokenized, nr_epochs=nr_epochs, num_samples=num_samples)


if __name__ == "__main__":
    train_nulog_model()
