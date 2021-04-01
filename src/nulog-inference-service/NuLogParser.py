# Standard Library
import logging
import os
import time

# Third Party
import torch
import torch.nn as nn
from NuLogModel import *  # should improve this
from NuLogTokenizer import LogTokenizer
from torchvision import transforms

# constant
# tell torch using or not using GPU
using_GPU = True if torch.cuda.device_count() > 0 else False
if not using_GPU:
    os.environ["CUDA_VISIBLE_DEVICES"] = ""
else:
    os.environ["CUDA_VISIBLE_DEVICES"] = "0"
# class logparser


class LogParser:
    def __init__(
        self, indir, outdir, filters, k, log_format, logName="nulog_model_latest"
    ):
        self.path = indir
        self.savePath = outdir
        self.logName = logName
        self.filters = filters
        self.k = k
        self.df_log = None
        self.log_format = log_format
        self.tokenizer = LogTokenizer(filters)

        if not os.path.exists(self.savePath):
            os.makedirs(self.savePath)

    def num_there(self, s):
        digits = [i.isdigit() for i in s]
        return True if np.mean(digits) > 0.0 else False

    def train(
        self,
        data_tokenized,
        batch_size=32,
        mask_percentage=1.0,
        pad_len=64,
        N=1,
        d_model=256,
        dropout=0.1,
        lr=0.001,
        betas=(0.9, 0.999),
        weight_decay=0.005,
        nr_epochs=5,
        num_samples=0,
        step_size=100,
    ):
        self.mask_percentage = mask_percentage
        self.pad_len = pad_len
        self.batch_size = batch_size
        self.N = N
        self.d_model = d_model
        self.dropout = dropout
        self.lr = lr
        self.betas = betas
        self.weight_decay = weight_decay
        self.num_samples = num_samples
        self.nr_epochs = nr_epochs
        self.step_size = step_size

        print("learning rate : " + str(self.lr))
        # test!
        # data_tokenized = data_tokenized[:10]
        transform_to_tensor = transforms.Lambda(lambda lst: torch.tensor(lst))

        criterion = nn.CrossEntropyLoss()
        model = self.make_model(
            self.tokenizer.n_words,
            self.tokenizer.n_words,
            N=self.N,
            d_model=self.d_model,
            d_ff=self.d_model,
            dropout=self.dropout,
            max_len=self.pad_len,
        )

        if using_GPU:
            model.cuda()
        model_opt = torch.optim.Adam(
            model.parameters(),
            lr=self.lr,
            betas=self.betas,
            weight_decay=self.weight_decay,
        )

        if (self.logName + ".pt") in os.listdir(self.savePath):
            model_path = self.savePath + self.logName + ".pt"
            model.load_state_dict(torch.load(model_path))

        train_dataloader = self.get_train_dataloaders(
            data_tokenized, transform_to_tensor
        )
        # train if no model
        print(f"#######Training Model within {self.nr_epochs} epochs...######")
        for epoch in range(self.nr_epochs):
            model.train()
            print("Epoch", epoch)
            self.run_epoch(
                train_dataloader,
                model,
                SimpleLossCompute(model.generator, criterion, model_opt),
            )
            torch.save(model.state_dict(), self.savePath + self.logName + ".pt")
            # torch.save(model.state_dict(), self.savePath + self.logName + '.pt')

    def init_inference(
        self,
        batch_size=32,
        mask_percentage=1.0,
        pad_len=64,
        N=1,
        d_model=256,
        dropout=0.1,
        lr=0.001,
        betas=(0.9, 0.999),
        weight_decay=0.005,
        nr_epochs=5,
        num_samples=0,
        step_size=100,
    ):
        # training can share this init function
        self.mask_percentage = mask_percentage
        self.pad_len = pad_len
        self.batch_size = batch_size
        self.N = N
        self.d_model = d_model
        self.dropout = dropout
        self.lr = lr
        self.betas = betas
        self.weight_decay = weight_decay
        self.num_samples = num_samples
        self.nr_epochs = nr_epochs
        self.step_size = step_size

        # data_tokenized = data_tokenized[:10]
        self.transform_to_tensor = transforms.Lambda(lambda lst: torch.tensor(lst))

        self.criterion = nn.CrossEntropyLoss()
        self.model = self.make_model(
            self.tokenizer.n_words,
            self.tokenizer.n_words,
            N=self.N,
            d_model=self.d_model,
            d_ff=self.d_model,
            dropout=self.dropout,
            max_len=self.pad_len,
        )

        if using_GPU:
            self.model.cuda()
        self.model_opt = torch.optim.Adam(
            self.model.parameters(),
            lr=self.lr,
            betas=self.betas,
            weight_decay=self.weight_decay,
        )
        model_path = "{}{}.pt".format(self.savePath, self.logName)
        if using_GPU:
            self.model.load_state_dict(torch.load(model_path))
        else:
            self.model.load_state_dict(
                torch.load(model_path, map_location=torch.device("cpu"))
            )

    def predict(self, data_tokenized, output_prefix=""):
        start_time = time.time()
        test_dataloader = self.get_test_dataloaders(
            data_tokenized, self.transform_to_tensor
        )
        t1 = time.time()
        logging.info(
            ("--- generate dataloader of logs in %s seconds ---" % (t1 - start_time))
        )

        results = self.run_test(
            test_dataloader,
            self.model,
            SimpleLossCompute(self.model.generator, self.criterion, None, is_test=True),
        )

        t2 = time.time()

        data_words = []
        indices_from = []

        theta = 0.8  # the threshold
        anomaly_preds = []
        total_anomaly = 0

        tmp = []
        for i, (x, y, ind) in enumerate(results):
            tmp.append([x, y, ind])
            true_pred = 0
            total_count = 0
            c_rates = []
            for j in range(len(x)):
                # print(self.tokenizer.index2word[y[j]])
                if not self.num_there(self.tokenizer.index2word[y[j]]):  # k8s

                    if y[j] in x[j][-self.k :]:
                        true_pred += 1
                        data_words.append(
                            self.tokenizer.index2word[y[j]]
                        )  # I buy (masked (1)) apple
                    else:
                        data_words.append("<*>")
                    total_count += 1

                    if j == len(x) - 1 or ind[j] != ind[j + 1]:  # a new token
                        c_rates.append(true_pred / total_count)
                        true_pred = 0
                        total_count = 0

                else:
                    data_words.append("<*>")
            # correct_rate = true_pred / total_count
            # assert len(c_rates) == 10
            # if correct_rate < theta:
            #    a_pred = True
            #    total_anomaly += 1
            # else:
            #    a_pred = False
            for c in c_rates:
                if c < theta:
                    total_anomaly += 1

            counts = len(set(ind.numpy()))
            # assert counts == 10
            # for i in range(counts): #use combined results for now, can be split for each data point
            #    anomaly_preds.append(correct_rate)
            for correct_rate in c_rates:
                anomaly_preds.append(correct_rate)
            indices_from += ind.tolist()

        logging.info(
            ("--- merged results of preds in %s seconds ---" % (time.time() - t2))
        )

        return anomaly_preds

    def save_parsed_logs(self, parsed_logs):
        df_event = self.outputResult(parsed_logs)
        df_event.to_csv(self.savePath + self.logName + "_structured.csv", index=False)

    def get_train_dataloaders(self, data_tokenized, transform_to_tensor):
        train_data = MaskedDataset(
            data_tokenized,
            self.tokenizer,
            mask_percentage=self.mask_percentage,
            transforms=transform_to_tensor,
            pad_len=self.pad_len,
        )
        weights = train_data.get_sample_weights()
        if self.num_samples != 0:
            train_sampler = WeightedRandomSampler(
                weights=list(weights), num_samples=self.num_samples, replacement=True
            )
        if self.num_samples == 0:
            train_sampler = RandomSampler(train_data)
        train_dataloader = DataLoader(
            train_data, sampler=train_sampler, batch_size=self.batch_size
        )
        return train_dataloader

    def get_test_dataloaders(self, data_tokenized, transform_to_tensor):
        test_data = MaskedDataset(
            data_tokenized,
            self.tokenizer,
            mask_percentage=self.mask_percentage,
            transforms=transform_to_tensor,
            pad_len=self.pad_len,
        )
        test_sampler = SequentialSampler(test_data)
        test_dataloader = DataLoader(
            test_data, sampler=test_sampler, batch_size=self.batch_size
        )
        return test_dataloader

    def load_data(self, logName):
        headers, regex = self.generate_logformat_regex(self.log_format)
        df_log = self.log_to_dataframe(
            os.path.join(self.path, logName), regex, headers, self.log_format
        )
        return [df_log.iloc[i].Content for i in range(df_log.shape[0])]

    def tokenize_data(self, input_text, isTrain=False):
        data_tokenized = []
        for i in trange(0, len(input_text)):
            text_i = input_text[i].lower()
            tokenized = self.tokenizer.tokenize("<CLS> " + text_i, isTrain=isTrain)
            data_tokenized.append(tokenized)
        return data_tokenized

    def log_to_dataframe(self, log_file, regex, headers, logformat):
        """Function to transform log file to dataframe"""
        log_messages = []
        linecount = 0
        with open(log_file, "r") as fin:
            for line in fin.readlines():
                try:
                    match = regex.search(line.strip())
                    message = [match.group(header) for header in headers]
                    log_messages.append(message)
                    linecount += 1
                except Exception as e:
                    pass
        logdf = pd.DataFrame(log_messages, columns=headers)
        logdf.insert(0, "LineId", None)
        logdf["LineId"] = [i + 1 for i in range(linecount)]
        return logdf

    def generate_logformat_regex(self, logformat):
        """Function to generate regular expression to split log messages"""
        headers = []
        splitters = re.split(r"(<[^<>]+>)", logformat)
        regex = ""
        for k in range(len(splitters)):
            if k % 2 == 0:
                splitter = re.sub(" +", "\\\s+", splitters[k])
                regex += splitter
            else:
                header = splitters[k].strip("<").strip(">")
                regex += "(?P<%s>.*?)" % header
                headers.append(header)
        regex = re.compile("^" + regex + "$")
        return headers, regex

    def do_mask(self, batch):
        c = copy.deepcopy
        token_id = self.tokenizer.word2index["<MASK>"]
        srcs, offsets, data_lens, indices = batch
        src, trg, idxs = [], [], []

        for i, _ in enumerate(data_lens):
            data_len = c(data_lens[i].item())
            # print(data_lens[i].item())
            dg = c(indices[i].item())
            masked_data = c(srcs[i])
            offset = offsets[i].item()
            num_masks = round(self.mask_percentage * data_len)
            if self.mask_percentage < 1.0:
                masked_indices = np.random.choice(
                    np.arange(offset, offset + data_len),
                    size=num_masks if num_masks > 0 else 1,
                    replace=False,
                )
            else:
                masked_indices = np.arange(offset, offset + data_len)

            masked_indices.sort()

            for j in masked_indices:
                tmp = c(masked_data)
                label_y = c(tmp[j])
                tmp[j] = token_id
                src.append(c(tmp))

                trg.append(label_y)
                idxs.append(dg)
        return torch.stack(src), torch.stack(trg), torch.Tensor(idxs)

    def run_epoch(self, dataloader, model, loss_compute):

        start = time.time()
        total_tokens = 0
        total_loss = 0
        tokens = 0
        for i, batch in enumerate(dataloader):

            b_input, b_labels, _ = self.do_mask(batch)
            batch = Batch(b_input, b_labels, 0)
            if using_GPU:
                out = model.forward(
                    batch.src.cuda(),
                    batch.trg.cuda(),
                    batch.src_mask.cuda(),
                    batch.trg_mask.cuda(),
                )

                loss = loss_compute(out, batch.trg_y.cuda(), batch.ntokens)
            else:
                out = model.forward(
                    batch.src, batch.trg, batch.src_mask, batch.trg_mask
                )

                loss = loss_compute(out, batch.trg_y, batch.ntokens)

            total_loss += loss
            total_tokens += batch.ntokens
            tokens += batch.ntokens

            if i % self.step_size == 1:
                elapsed = time.time() - start
                print(
                    "Epoch Step: %d / %d Loss: %f Tokens per Sec: %f"
                    % (i, len(dataloader), loss / batch.ntokens, tokens / elapsed)
                )
                start = time.time()
                tokens = 0
        return total_loss / total_tokens

    def run_test(self, dataloader, model, loss_compute):
        # Standard Library
        import time

        model.eval()
        if using_GPU:
            model.cuda()
        with torch.no_grad():
            for i, batch in enumerate(dataloader):
                b_input, b_labels, ind = self.do_mask(batch)

                batch = Batch(b_input, b_labels, 0)
                if using_GPU:
                    out = model.forward(
                        batch.src.cuda(),
                        batch.trg.cuda(),
                        batch.src_mask.cuda(),
                        batch.trg_mask.cuda(),
                    )
                else:
                    out = model.forward(
                        batch.src, batch.trg, batch.src_mask, batch.trg_mask
                    )

                out_p = model.generator(out)  # batch_size, hidden_dim
                t3 = time.perf_counter()

                # if i % self.step_size == 1:
                #     print("Epoch Step: %d / %d" % (i, len(dataloader)))
                # r1 = out_p.cpu().numpy().argsort(axis=1) # this is why it's so slow
                r11 = torch.argsort(out_p, dim=1)
                r1 = r11.cpu().numpy()
                r2 = b_labels.cpu().numpy()
                r3 = ind.cpu()
                yield r1, r2, r3
                t4 = time.perf_counter()
                # print(t4 - t3)

    def outputResult(self, pred):
        df_events = []
        templateids = []
        for pr in pred:
            template_id = hashlib.md5(pr.encode("utf-8")).hexdigest()
            templateids.append(template_id)
            df_events.append([template_id, pr])

        df_event = pd.DataFrame(df_events, columns=["EventId", "EventTemplate"])
        return df_event

    def make_model(
        self,
        src_vocab,
        tgt_vocab,
        N=3,
        d_model=512,
        d_ff=2048,
        h=8,
        dropout=0.1,
        max_len=20,
    ):
        "Helper: Construct a model from hyperparameters."
        c = copy.deepcopy
        attn = MultiHeadedAttention(h, d_model)
        ff = PositionwiseFeedForward(d_model, d_ff, dropout)
        position = PositionalEncoding(d_model, dropout, max_len)
        model = EncoderDecoder(
            Encoder(EncoderLayer(d_model, c(attn), c(ff), dropout), N),
            Decoder(DecoderLayer(d_model, c(attn), c(attn), c(ff), dropout), N),
            nn.Sequential(Embeddings(d_model, src_vocab), c(position)),
            nn.Sequential(Embeddings(d_model, tgt_vocab), c(position)),
            Generator(d_model, tgt_vocab),
        )

        # This was important from their code.
        # Initialize parameters with Glorot / fan_avg.
        for p in model.parameters():
            if p.dim() > 1:
                nn.init.xavier_uniform(p)
        return model
