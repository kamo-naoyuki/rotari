rotari Python client
====================

The Python package is a thin subprocess wrapper around the ``rotari``
executable. It submits executable argument lists rather than Python functions.

Quick start
-----------

.. code-block:: python

   from rotari import Rotari

   rotari = Rotari(basedir=".rotari-state", project="experiment")
   rotari.add(["./train.sh"], job_name="train")
   rotari.run(async_=True)
   summary = rotari.wait()

``wait()`` and ``show()`` decode rotari's JSON output into dictionaries. Other
commands return :class:`rotari.CommandResult`; command failures raise
:class:`rotari.RotariError`.

The command methods accept the options shown below. Their signatures are
created from rotari's checked-in CLI schema, so they update when that schema is
regenerated.

.. toctree::
   :maxdepth: 2

   api
