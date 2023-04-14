
import matplotlib.pyplot as plt
import numpy.random as random
from sklearn.preprocessing import minmax_scale

def plt_plot(ts_data):

    # Plot the time series data
    plt.plot(ts_data)
    plt.title("Simulated Time Series Data")
    plt.xlabel("Time")
    plt.ylabel("Value")
    plt.show()

def normalize_timeseries(ts_data):
    return minmax_scale(ts_data)