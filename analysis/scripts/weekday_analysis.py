import itertools
import matplotlib.pyplot as plt
import csv
from datetime import datetime, date

import pytz
import matplotlib.pyplot as plt


CSV_FILE_PATH = "../data/queueReports.csv"


def load_csv():
    """Reads the given csv file and returns an array of tuples of (time/length)
    expects the csv to be structures as
    timestamp, length measurement
    """
    array_of_measurements = []

    with open(CSV_FILE_PATH) as csv_file:
        csv_reader = csv.reader(csv_file)
        for row in csv_reader:
            if row[0].isdigit():
                array_of_measurements.append((int(row[0]), row[1]))
    return array_of_measurements


def sort_into_weekdays(array_of_measurements):
    """Expects a list of all measurements made.
    Returns a list with seven elements, each representing a weekday,
    starting with monday.
    Each of these elements is a list itself, think bucket,
    that contains all measurements that were made on that weekday"""
    day_buckets = []
    # order is monday, tuesday, ..., saturday, sunday
    for i in range(7):
        day_buckets.append([])


    for measurement in array_of_measurements:
        timestamp = datetime.fromtimestamp(measurement[0])
        weekday_of_measurement = timestamp.weekday() # weekdays are from 0 to 6
        day_buckets[weekday_of_measurement].append(measurement)

    return day_buckets


def normalize_measurement_tuple(m_tuple):
    """Given a tuple of (timestamp, string), this
    normalizes the timestamp to include just time, not date, and the
    string to an int.
    All timestamps returned by this function keep their time,
    but now happen on the same day.
    For the labels each LX value is converted to the appropriate X"""
    # Normalize Time
    time = datetime.fromtimestamp(m_tuple[0], tz=pytz.timezone('Europe/Berlin')).time()
    time_normalized = datetime.combine(date(2022,1,1), time)
    # Normalize Label
    label_as_int = int(m_tuple[1][1])

    return (time_normalized, label_as_int)

def get_list_of_mensa_opening_times():
    """Returns a list containing datetime objects,
    one for each hour the mensa is open"""
    opening_times = []
    hour_counter = 8
    for i in range(8):
        opening_times.append(datetime(2022,1,1,hour_counter+i))

    return opening_times


def get_day_of_week_string_from_measurement(measurement):
    """Expects a measurement. Returns a string naming
    the weekday on which that measurement was taken"""
    days = ["Monday", "Tuesday", "Wednesday",
        "Thursday", "Friday", "Saturday", "Sunday"]
    weekday_number = datetime.fromtimestamp(measurement[0]).weekday()
    return days[weekday_number]


def illustrate_single_weekday(list_of_measurement_tuples):
    """Gets a list of touples containing measurements of a single
    day. Creates a graph for those measurements"""

    tuples_to_illustrate = []
    x_axis_points = []
    y_axis_points = []
    day_of_week = get_day_of_week_string_from_measurement(list_of_measurement_tuples[0])

    # Normalize all measurements
    for measurement in list_of_measurement_tuples:
        normalized_tuple = normalize_measurement_tuple(measurement)
        if(normalized_tuple[0].hour < 15 and normalized_tuple[0].hour > 8): # End of mensa opening time
            tuples_to_illustrate.append(normalized_tuple)



    # Matplotlib requires two sorted lists, so create those
    tuples_to_illustrate.sort(key=lambda t: t[0])
    for measurement in tuples_to_illustrate:
        x_axis_points.append(measurement[0])
        y_axis_points.append(measurement[1])
    

    y_ticks_labels = ["L0: Virtually empty", "L1: Within kitchen", "L2: Up to food trays", "L3: Within first room", "L4: Starting to corner", "L5: Past first desk", "L6: Past second desk", "L7: Up to stairs", "L8: Even longer"]
    x_ticks_labels = ["08:00", "09:00", "10:00", "11:00", "12:00", "13:00", "14:00", "15:00"]

    figure, ax= plt.subplots()

    ax.set_xticks(get_list_of_mensa_opening_times())
    ax.set_xticklabels(x_ticks_labels)
    ax.set_yticks([0,1,2,3,4,5,6,7,8])
    ax.set_yticklabels(y_ticks_labels)
    ax.plot(x_axis_points, y_axis_points)

    # plt.show()
    plt.title(day_of_week)
    plt.tight_layout()
    plt.savefig(day_of_week + ".png", format='png', dpi=200)
        




def main():
    all_measurements = load_csv()
    grouped_by_weekday = sort_into_weekdays(all_measurements)
    for i in range(5):
        illustrate_single_weekday(grouped_by_weekday[i])


if __name__=="__main__":
    main()
