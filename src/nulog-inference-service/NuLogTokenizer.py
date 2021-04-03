# Standard Library
import logging
import os


class LogTokenizer:
    def __init__(self, filters="([ |:|\(|\)|=|,])|(core.)|(\.{2,})"):
        self.filters = filters
        self.word2index = {"<PAD>": 0, "<CLS>": 1, "<MASK>": 2, "<UNK>": 3, "<NUM>": 4}
        self.index2word = {0: "<PAD>", 1: "<CLS>", 2: "<MASK>", 3: "<UNK>", 4: "<NUM>"}
        self.n_words = 10000  # Count SOS and EOS
        self.valid_words = 5
        for i in range(self.valid_words, self.n_words):
            tmpword = "<TMP" + str(i) + ">"
            self.word2index[tmpword] = i
            self.index2word[i] = tmpword
        # self.regex_masker = masker()

    def addWord(self, word):
        if word not in self.word2index and self.valid_words < self.n_words:
            self.word2index[word] = self.valid_words
            self.index2word[self.valid_words] = word
            self.valid_words += 1

    def load_vocab(self, filepath):
        self.word2index = {}
        self.index2word = {}
        logging.info("load vocab........")
        with open(os.path.join(filepath, "vocab.txt"), "r") as fin:
            self.n_words = int(fin.readline().rstrip())
            self.valid_words = int(fin.readline().rstrip())
            logging.info("n_words : " + str(self.n_words))
            logging.info("valid_words : " + str(self.valid_words))
            for idx, line in enumerate(fin):
                word_i = line.replace("\n", "")
                self.index2word[idx] = word_i
                self.word2index[word_i] = idx

    def save_vocab(self, filepath):
        with open(os.path.join(filepath, "vocab.txt"), "w") as fout:
            logging.info("n_words : " + str(self.n_words))
            logging.info("valid_words : " + str(self.valid_words))
            fout.write(str(self.n_words))
            fout.write("\n")
            fout.write(str(self.valid_words))
            fout.write("\n")
            for n in range(self.n_words):
                fout.write(self.index2word[n])
                fout.write("\n")

    def tokenize(self, sent, isTrain):
        tokens = sent.split(" ")
        # sent = sent.replace('\'', '')
        # sent = sent.replace('\"', '')
        # #sent = self.regex_masker.mask(sent)
        # filtered = re.split(self.filters, sent)

        res = []
        for word in tokens:
            if isTrain:
                self.addWord(word)
            if word in self.word2index:
                res.append(self.word2index[word])
            else:
                res.append(self.word2index["<UNK>"])
        return res
