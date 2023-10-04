import threading
import time

def fn_simple():
    print("Thread simple: starting")
    time.sleep(2)
    print("Thread simple: finishing")

def fn_arg(name):
    print("Thread %s: starting", name)
    time.sleep(2)
    print("Thread %s: finishing", name)

    print("Thread   : ct => ", threading.current_thread())

if __name__ == "__main__":
    print("Main    : before creating thread")
    x = threading.Thread(target=fn_arg, args=(1,))
    #x = threading.Thread(target=fn_simple)
    print("Main    : before running thread")
    print("Main    : is alive => ", x.is_alive())
    x.start()

    print("Main    : wait for the thread to finish")
    x.join()
    print("Main    : all done")

    print("Main    : is alive => ", x.is_alive())

    threading.current_thread()
