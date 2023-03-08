from model import MpcModel
from data_simulator import simulate_data
import torch
import torch.nn as nn
from torch.utils.data import Dataset, DataLoader
from torch.optim import Adam, SGD
import numpy as np

device = torch.device("cuda" if torch.cuda.is_available() else 'cpu')


def train():
    batch_size = 32
    train_data = simulate_data(1000)
    train_loader = DataLoader(train_data, batch_size=batch_size, shuffle=True)
    total_batch = len(train_loader)
    n_epoch = 400 
    learning_rate = 0.001
    weight_decay = 0.0005 #1e-4, 1e-3, 1e-2

    seed = 1234
    np.random.seed(seed)

    model = MpcModel() # random initialization.
    model = model.to(device)
    optimizer = torch.optim.Adam(model.parameters(), lr = learning_rate, weight_decay=weight_decay)
    loss_fn = nn.CrossEntropyLoss()

    model.train()
    for e in range(n_epoch):
        print(f"epoch : {e}")
        total_loss = 0
        model.train()
        for i, (x_batch, y_batch) in enumerate(train_loader):
            optimizer.zero_grad()
            y_pred = model(x_batch.to(device))
            loss = loss_fn(y_pred.cpu(), y_batch)
            # optimizer.zero_grad()
            loss.backward()
            
            optimizer.step()
            total_loss += loss.item()
        
        print(f"total loss : {total_loss}, average loss : {total_loss / total_batch}")

        ## eval?

    torch.save(model.state_dict() ,"model.pth")
    return model

def eval():
    test_data = simulate_data(100)
    test_loader = DataLoader(test_data, batch_size=1, shuffle=False)
    num_correct, num_total = 0, 0
    mistakes = []
    model = MpcModel() # random initialization.
    model = model.to(device)
    model.load_state_dict(torch.load("model.pth"))
    model.eval()

    with torch.no_grad():
        for i, (x_batch, y_batch) in enumerate(test_loader):
        y_pred = model(x_batch.to(device))
        y_pred = torch.argmax(y_pred, dim=1).cpu()
        nc = torch.sum(y_pred == y_batch).item()
        num_correct += nc
        nt = y_batch.size(0)
        num_total += nt
        if nc == 0:
            mistakes.append(i)

    accuracy = num_correct / num_total
    print(f"accuracy : {accuracy}")
    print(f"mistakes : {mistakes}")
    print(len(mistakes))
    print(num_total)