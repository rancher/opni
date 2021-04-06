# Standard Library
import logging

# Third Party
from NuLogParser import LogParser

log_format = "<Content>"
filters = '([ |:|\(|\)|\[|\]|\{|\}|"|,|=])'


def init_model():
    logging.info("initializing...")
    k = 50  # was 50 ## tunable, top k predictions
    nr_epochs = 1
    num_samples = 0

    output_dir = "output/"
    parser = LogParser(filters=filters, k=k, log_format=log_format)
    parser.tokenizer.load_vocab()
    parser.init_inference(nr_epochs=nr_epochs, num_samples=num_samples)

    return parser


def predict(parser, texts):
    tokenized = parser.tokenize_data(texts, isTrain=False)
    preds = parser.predict(tokenized)
    return preds


def main():
    parser = init_model()
    test_texts = ["testing sentence for inference!"]
    preds = predict(parser, test_texts)
    logging.info(preds)


if __name__ == "__main__":
    main()
