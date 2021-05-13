# Standard Library
import logging

# Third Party
from NuLogParser import LogParser


def init_model(save_path="output/"):
    logging.info("initializing...")
    nr_epochs = 1
    num_samples = 0

    parser = LogParser(save_path=save_path)
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
