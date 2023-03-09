import torch
import torch.nn as nn
from torch.utils.data import Dataset, DataLoader
from torch.optim import Adam, SGD

class MpcDataset(Dataset):
  def __init__(self, x, y):
    self.x = x
    self.y = y
  def __len__(self):
    return len(self.x)
  def __getitem__(self, idx):
    return self.x[idx], self.y[idx]
  

# the model
class MpcModel(nn.Module):
    
    def __init__(self,n_class = 13):     
        self.n_class = n_class
        ts_len = 60
        c_in = 1
        c_out0 = 64
        c_out1 = 128
        c_out2 = 256
        k = 5
        p = 2
        l_out0 = 64
        self.l_in = ts_len // 2 * c_out2

        super(MpcModel, self).__init__()
        self.conv0 = nn.Conv1d(c_in, c_out0, k, padding=p)
        self.relu0 = nn.ReLU()

        self.conv1 = nn.Conv1d(c_out0, c_out1, k, padding=p)
        self.relu1 = nn.ReLU()

        self.conv2 = nn.Conv1d(c_out1, c_out2, k, padding=p)
        self.relu2 = nn.ReLU()
        self.pool2 = nn.MaxPool1d(2)

        self.linear3 = nn.Linear(self.l_in, l_out0)
        self.dropout3 = nn.Dropout(p=0.5) 
        self.relu3 = nn.ReLU() 

        self.linear4 = nn.Linear(l_out0, n_class)
        self.relu4 = nn.ReLU()     

    def forward(self, x):
        x = self.conv0(x)
        x = self.relu0(x)

        x = self.conv1(x)
        x = self.relu1(x)

        x = self.conv2(x)
        x = self.relu2(x)
        x = self.pool2(x)

        x = x.view(-1, self.l_in)

        x = self.linear3(x)
        x = self.dropout3(x)
        x = self.relu3(x)

        x = self.linear4(x)
        x = self.relu4(x)
        return x
