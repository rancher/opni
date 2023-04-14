import torch
import numpy as np
import matplotlib.pyplot as plt
import numpy.random as random
from sklearn.preprocessing import minmax_scale
from model.cnn_model import MpcDataset
from model.utils import normalize_timeseries

n = 60
min_shift_n = 5
max_shift_n = 55
min_n = 1
max_n = 100
sigma = 1
min_sigma_increase = 6

class DataSimulator:

    @staticmethod
    def get_class_map():
      return {
      0: DataSimulator.type1_level_shift_up,
      1: DataSimulator.type1_level_shift_down,
      2: DataSimulator.type1_steady_increase,
      3:DataSimulator.type1_steady_decrease,
      4: DataSimulator.type1_sudden_increase,
      5:DataSimulator.type1_sudden_decrease,
      6: DataSimulator.type2_single_spike,
      7: DataSimulator.type2_single_dip,
      8: DataSimulator.type2_multi_spike,
      9:DataSimulator.type2_multi_dip,
      10:DataSimulator.type2_transient_level_up,
      11:DataSimulator.type2_transient_level_down,
      12: DataSimulator.type2_fluctuations,
    }

    @staticmethod
    def steady_fluctuate_line():
        # Define parameters
        mu = np.random.uniform(min_n,max_n)

        # Generate time series data
        ts_data = np.random.normal(mu, sigma, n)
        return ts_data

    @staticmethod
    def type1_sudden_increase():
        increase_len = random.randint(1, 4)

        mu = np.random.uniform(min_n,max_n)
        ts_data = np.random.normal(mu, sigma, n)
        total_increase = random.uniform(6, 50) * sigma
        base = mu + sigma
        for j in range(increase_len):
            i = j - increase_len 
            if i != -1:
                this_increase = random.uniform(0, total_increase)
                ts_data[i] = base + this_increase
                base = ts_data[i]
                total_increase -= this_increase
            else:
                ts_data[i] = base + total_increase
        
        return ts_data

    @staticmethod
    def type1_sudden_decrease():
        increase_len = random.randint(1, 4)

        mu = np.random.uniform(min_n,max_n)
        ts_data = np.random.normal(mu, sigma, n)
        total_increase = random.uniform(6, 50) * sigma
        base = mu - sigma
        for j in range(increase_len):
            i = j - increase_len 
            if i != -1:
                this_increase = random.uniform(0, total_increase)
                ts_data[i] = base - this_increase
                base = ts_data[i]
                total_increase -= this_increase
            else:
                ts_data[i] = base - total_increase
        
        return ts_data

    @staticmethod
    def type1_level_shift_up():
        max_mu1 = (max_n - min_n) * 0.7 + min_n
        mu1 = random.uniform(min_n,max_mu1)
        mu2 = random.uniform(max_mu1 + min_sigma_increase * sigma, max_n)

        shift_point = random.randint(min_shift_n, max_shift_n)
        ts_data1 = random.normal(mu1, sigma, shift_point)
        ts_data2 = random.normal(mu2, sigma, n - shift_point)
        ts_data = np.concatenate((ts_data1, ts_data2), axis=0)
        assert len(ts_data) == n
        return ts_data


    @staticmethod
    def type1_level_shift_down():
        min_mu1 = (max_n - min_n) * 0.3 + min_n
        mu1 = random.uniform(min_mu1, max_n)
        mu2 = random.uniform(min_n , min_mu1 - min_sigma_increase * sigma)

        shift_point = random.randint(min_shift_n, max_shift_n)
        ts_data1 = random.normal(mu1, sigma, shift_point)
        ts_data2 = random.normal(mu2, sigma, n - shift_point)
        ts_data = np.concatenate((ts_data1, ts_data2), axis=0)
        assert len(ts_data) == n
        return ts_data

    @staticmethod
    def type1_steady_increase():
        max_mu1 = (max_n - min_n) * 0.7 + min_n
        mu1 = random.uniform(min_n,max_mu1)
        mu2 = random.uniform(max_mu1 + min_sigma_increase * sigma, max_n)

        # Generate time series data with a steady increase
        linear_increase = np.linspace(mu1, mu2, n)
        noise = np.random.normal(0, sigma, n)
        ts_data = linear_increase + noise
        return ts_data

    @staticmethod
    def type1_steady_decrease():
        min_mu1 = (max_n - min_n) * 0.3 + min_n
        mu1 = random.uniform(min_mu1, max_n)
        mu2 = random.uniform(min_n , min_mu1 - min_sigma_increase * sigma)

        # Generate time series data with a steady increase
        linear_increase = np.linspace(mu1, mu2, n)
        noise = np.random.normal(0, sigma, n)
        ts_data = linear_increase + noise
        return ts_data
            
    @staticmethod
    def type2_single_spike():
        mu = np.random.uniform(min_n,max_n)

        # Generate time series data
        ts_data = np.random.normal(mu, sigma, n)
        spike_n = random.randint(min_shift_n, n - 1)
        ts_data[spike_n] = random.uniform(min_sigma_increase, 30) * sigma + mu + sigma
        return ts_data

    @staticmethod
    def type2_single_dip():
        mu = np.random.uniform(min_n,max_n)

        # Generate time series data
        ts_data = np.random.normal(mu, sigma, n)
        spike_n = random.randint(min_shift_n, n - 1)
        ts_data[spike_n] = -random.uniform(min_sigma_increase, 30) * sigma + mu- sigma
        return ts_data

    @staticmethod
    def type2_multi_spike():
        mu = np.random.uniform(min_n,max_n)

        # Generate time series data
        ts_data = np.random.normal(mu, sigma, n)

        num_extra_spikes = random.randint(2,5)
        spike_1 = random.randint(min_shift_n, n - 1 - 2) # -2 to leave room for extra spike
        ts_data[spike_1] = random.uniform(min_sigma_increase, 30) * sigma + mu + sigma
        new_min_n = spike_1 + 2
        for i in range(num_extra_spikes):
            if new_min_n < n - 1:
                this_spike = random.randint(new_min_n, n - 1)
                ts_data[this_spike] = random.uniform(min_sigma_increase, 30) * sigma + mu + sigma
                new_min_n = this_spike + 2
        return ts_data

    @staticmethod
    def type2_multi_dip():
        mu = np.random.uniform(min_n,max_n)

        # Generate time series data
        ts_data = np.random.normal(mu, sigma, n)

        num_extra_spikes = random.randint(2,5)
        spike_1 = random.randint(min_shift_n, n - 1 - 2) # -2 to leave room for extra spike
        ts_data[spike_1] = -random.uniform(min_sigma_increase, 30) * sigma + mu + -sigma
        new_min_n = spike_1 + 2
        for i in range(num_extra_spikes):
            if new_min_n < n - 1:
                this_spike = random.randint(new_min_n, n - 1)
                ts_data[this_spike] = -random.uniform(min_sigma_increase, 30) * sigma + mu + -sigma
                new_min_n = this_spike + 2
        return ts_data

    @staticmethod
    def type2_transient_level_up():
        max_mu1 = (max_n - min_n) * 0.7 + min_n
        mu1 = random.uniform(min_n,max_mu1)
        mu2 = random.uniform(max_mu1 + min_sigma_increase * sigma, max_n)

        shift_point = random.randint(min_shift_n, max_shift_n)
        shift_len = random.randint(2, n-1-shift_point)
        ts_data = random.normal(mu1, sigma, n)
        ts_data2 = random.normal(mu2, sigma, shift_len)
        ts_data[shift_point: shift_point + shift_len] = ts_data2
        return ts_data

    @staticmethod
    def type2_transient_level_down():
        min_mu1 = (max_n - min_n) * 0.3 + min_n
        mu1 = random.uniform(min_mu1, max_n)
        mu2 = random.uniform(min_n , min_mu1 - min_sigma_increase * sigma)

        shift_point = random.randint(min_shift_n, max_shift_n)
        shift_len = random.randint(2, n-1-shift_point)
        ts_data = random.normal(mu1, sigma, n)
        ts_data2 = random.normal(mu2, sigma, shift_len)
        ts_data[shift_point: shift_point + shift_len] = ts_data2
        return ts_data

    @staticmethod
    def type2_fluctuations():
        # Define parameters
        mu = np.random.uniform(min_n,max_n)
        multiplier_sigma = random.randint(min_sigma_increase, 30)

        # Generate time series data
        ts_data = np.random.normal(mu, multiplier_sigma * sigma, n)
        return ts_data




def simulate_data(n):
  pattern_class_map = DataSimulator.get_class_map()
  data = []
  label = []

  for c in pattern_class_map:
    f = pattern_class_map[c]
    for i in range(n):
      normalized_data = [normalize_timeseries(f())]
      normalize_data = torch.tensor(np.array(normalized_data), dtype=torch.float32)
      data.append(normalized_data)
      label.append(c)
  return MpcDataset(data, label)

