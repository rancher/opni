from model.cnn_model import MpcModel, MpcDataset
from model.data_simulator import simulate_data 
from model.utils import normalize_timeseries
import torch
import torch.nn as nn
from torch.utils.data import Dataset, DataLoader
import numpy as np
from envvars import MODEL_PATH
from typing import List

gpu_available = torch.cuda.is_available()
device = torch.device("cuda" if gpu_available else 'cpu')


class_map = {
      0: "type1_level_shift_up",
      1: "type1_level_shift_down",
      2: "type1_steady_increase",
      3: "type1_steady_decrease",
      4: "type1_sudden_increase",
      5:"type1_sudden_decrease",
      6: "type2_single_spike",
      7: "type2_single_dip",
      8: "type2_multi_spike",
      9:"type2_multi_dip",
      10:"type2_transient_level_up",
      11:"type2_transient_level_down",
      12: "type2_fluctuations",
}

def train_model():
    batch_size = 32
    train_x, train_y = simulate_data(1000)
    train_data = MpcDataset(train_x, train_y) 
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

    torch.save(model.state_dict() ,MODEL_PATH)
    return model

def eval_model(test_x = None, test_y = None, simulate_data_n: int=100):
    '''
    evaluate trained model. Should only be invoked after model training
    test_x: a list of torch.Tensor() or numpy array
    '''
    if test_x is None:
        test_x, test_y = simulate_data(simulate_data_n)
    test_x = [torch.tensor(np.array([normalize_timeseries(t)]), dtype=torch.float32) for t in test_x] 
    test_data = MpcDataset(test_x, test_y)
    test_loader = DataLoader(test_data, batch_size=1, shuffle=False)
    num_correct, num_total = 0, 0
    mistakes = []
    model = MpcModel()
    model = model.to(device)
    if gpu_available:
        model.load_state_dict(torch.load(MODEL_PATH))
    else:
        model.load_state_dict(torch.load(MODEL_PATH , map_location=torch.device('cpu')))
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
    return accuracy

def predict(pred_data: List[List[float]]) -> List[str]:
    '''
    model prediction.
    Input:
    @pred_data: List of timeseries. 
    '''
    # shape the data as model requires -- (n , 1 , 60)
    pred_data = [torch.tensor(np.array([normalize_timeseries(p)]), dtype=torch.float32) for p in pred_data] 

    # predict
    test_loader = DataLoader(pred_data, batch_size=1, shuffle=False)
    model = MpcModel()
    model = model.to(device)
    if gpu_available:
        model.load_state_dict(torch.load(MODEL_PATH))
    else:
        model.load_state_dict(torch.load(MODEL_PATH , map_location=torch.device('cpu')))
    model.eval()
    res = []

    with torch.no_grad():
        for i, x_batch in enumerate(test_loader):
            y_pred = model(x_batch.to(device))
            y_pred = torch.argmax(y_pred, dim=1).cpu()
            res.append(class_map[int(y_pred)])
    return res