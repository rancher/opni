# Standard Library
import logging
import os
import re
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
logging.basicConfig(level=logging.INFO, format="%(asctime)s - %(name)s - %(message)s")


class LogParser:
    def __init__(
        self,
        k=50,
        log_format="<Content>",
        model_name="nulog_model_latest.pt",
        save_path="output/",
    ):
        self.savePath = save_path
        self.k = k
        self.df_log = None
        self.log_format = log_format
        self.tokenizer = LogTokenizer(self.savePath)

        if not os.path.exists(self.savePath):
            os.makedirs(self.savePath)
        self.model_name = model_name
        self.model_path = os.path.join(self.savePath, self.model_name)

    def num_there(self, s):
        digits = [i.isdigit() for i in s]
        return True if np.mean(digits) > 0.0 else False

    def save_model(self, model, model_opt, epoch, loss):
        torch.save(
            {
                "epoch": epoch,
                "model_state_dict": model.state_dict(),
                "optimizer_state_dict": model_opt.state_dict(),
                "loss": loss,
            },
            self.model_path,
        )

    def load_model(self, model, model_opt):
        if using_GPU:
            ckpt = torch.load(self.model_path)
        else:
            ckpt = torch.load(self.model_path, map_location=torch.device("cpu"))
        try:
            model_opt.load_state_dict(ckpt["optimizer_state_dict"])
            model.load_state_dict(ckpt["model_state_dict"])
            epoch = ckpt["epoch"]
            loss = ckpt["loss"]
            return epoch, loss
        except:  ## TODO: remove this try except when we use the new save function.
            logging.warning("loading trained model with old format.")
            model.load_state_dict(ckpt)

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

        logging.debug("learning rate : " + str(self.lr))
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

        if (self.model_name) in os.listdir(self.savePath):
            # model.load_state_dict(torch.load(self.model_path))
            prev_epoch, prev_loss = self.load_model(model, model_opt)

        train_dataloader = self.get_train_dataloaders(
            data_tokenized, transform_to_tensor
        )
        ## train if no model
        model.train()
        logging.info("#######Training Model within {self.nr_epochs} epochs...######")
        for epoch in range(self.nr_epochs):
            logging.info("Epoch: {}".format(epoch))
            self.run_epoch(
                train_dataloader,
                model,
                SimpleLossCompute(model.generator, criterion, model_opt),
            )

        self.save_model(model=model, model_opt=model_opt, epoch=self.nr_epochs, loss=0)
        # torch.save(model.state_dict(), "nulog_model_latest.pt")

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
        self.load_model(self.model, self.model_opt)
        self.model.eval()

    def predict(self, data_tokenized, output_prefix=""):
        test_dataloader = self.get_test_dataloaders(
            data_tokenized, self.transform_to_tensor
        )

        results = self.run_test(
            test_dataloader,
            self.model,
            SimpleLossCompute(self.model.generator, self.criterion, None, is_test=True),
        )

        anomaly_preds = []
        for i, (x, y, ind) in enumerate(results):
            true_pred = 0
            total_count = 0
            c_rates = []
            for j in range(len(x)):
                if not self.num_there(self.tokenizer.index2word[y[j]]):

                    if y[j] in x[j][-self.k :]:  ## if it's within top k predictions
                        true_pred += 1
                    total_count += 1

                if j == len(x) - 1 or ind[j] != ind[j + 1]:
                    this_rate = 1.0 if total_count == 0 else true_pred / total_count
                    c_rates.append(this_rate)
                    true_pred = 0
                    total_count = 0

            anomaly_preds.extend(c_rates)

        return anomaly_preds

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

    def load_data(self, windows_folder_path):
        headers, regex = self.generate_logformat_regex(self.log_format)
        df_log = self.log_to_dataframe(
            windows_folder_path, regex, headers, self.log_format
        )
        return [df_log.iloc[i].Content for i in range(df_log.shape[0])]

    def tokenize_data(self, input_text, isTrain=False):
        data_tokenized = []
        for i in range(0, len(input_text)):
            text_i = input_text[i].lower()  ## lowercase before tokenization
            tokenized = self.tokenizer.tokenize("<CLS> " + text_i, isTrain=isTrain)
            data_tokenized.append(tokenized)
        return data_tokenized

    def log_to_dataframe(self, windows_folder_path, regex, headers, logformat):
        """Function to transform log file to dataframe"""
        all_log_messages = []
        json_files = sorted(
            [
                file
                for file in os.listdir(windows_folder_path)
                if file.endswith(".json.gz")
            ]
        )
        for window_file in json_files:
            window_df = pd.read_json(
                os.path.join(windows_folder_path, window_file), lines=True
            )
            masked_log_messages = window_df["masked_log"].tolist()
            regex_log_messages = []
            for masked_message in masked_log_messages:
                try:
                    match = regex.search(masked_message)
                    message = [match.group(header) for header in headers]
                    regex_log_messages.append(message)
                except Exception as e:
                    pass
            all_log_messages.extend(regex_log_messages)
        logdf = pd.DataFrame(all_log_messages, columns=headers)
        logdf.insert(0, "LineId", None)
        logdf["LineId"] = [i + 1 for i in range(len(all_log_messages))]
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
                logging.info(
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

                if i % self.step_size == 1:
                    logging.debug("Epoch Step: %d / %d" % (i, len(dataloader)))
                # r1 = out_p.cpu().numpy().argsort(axis=1) # this is why it's so slow
                r11 = torch.argsort(out_p, dim=1)
                r1 = r11.cpu().numpy()
                r2 = b_labels.cpu().numpy()
                r3 = ind.cpu()
                yield r1, r2, r3
                t4 = time.perf_counter()

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
